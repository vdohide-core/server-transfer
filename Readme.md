# Server Transfer

Worker service สำหรับโหลดไฟล์จาก S3 ingests ลง local storage และบันทึก media records สำหรับ [VDOHide](https://vdohide.com)

ติดตั้งบน **เครื่องเดียวกับ `server-storage`** — ไม่ encode วิดีโอ (ไม่ต้องใช้ FFmpeg)

## Pipeline

```
server-download  →  S3 temp (ingests)  →  ready_original
       ↓
server-transfer  →  local STORAGE_PATH  →  medias + ready
       ↓
server-transcode →  encode 1080/720/480/360 + sprite (บนเครื่อง storage)
```

`server-download` อัปโหลดแค่ `file_original.mp4` ขึ้น S3 — transfer โหลดลง disk แล้ว transcode ทำ rendition ที่เหลือภายหลัง

## Features

- **S3 → Local** — ดาวน์โหลดจาก S3 ingests (`sourceType: processed`) ลง `STORAGE_PATH`
- **Multi-resolution** — รองรับ `original`, `1080`, `720`, `480`, `360` (ถ้ามีบน S3)
- **Sprite** — ดาวน์โหลด `sprite.zip` แล้วแตกเป็น `sprite/`
- **Global media check** — เช็ค `medias` ตาม resolution ทั้งระบบ (ไม่สน `storageId`) ถ้ามีแล้วข้าม — กันโหลดซ้ำเมื่อไฟล์ถูก transfer ไป storage อื่นแล้ว
- **Soft-delete ingests** — หลังโหลดเสร็จ (หรือข้ามเพราะมี media แล้ว) ตั้ง `ingests.deletedAt` เพื่อให้ `server-service` ลบ object บน S3
- **Clone media** — สร้าง media record ให้ไฟล์ `clonedFrom` อัตโนมัติ
- **Multi-Worker** — รันหลาย instance พร้อมกัน ด้วย `WORKER_ID` ที่ไม่ซ้ำกัน
- **Auto Retry** — retry อัตโนมัติ 3 ครั้ง
- **Progress Tracking** — ติดตามใน `video_process` (`processType: transfer`)

## Requirements

- **MongoDB** (vdohide platform database)
- ติดตั้งคู่กับ **server-storage** บนเครื่องเดียวกัน
- S3 storage ใน database ต้อง `online` และ `accepts` มี `temp`

---

## Installation (Linux Server)

### One-line install

```bash
curl -fsSL https://raw.githubusercontent.com/vdohide-core/server-transfer/main/install.sh | sudo -E bash -s -- \
    --mongodb-uri "mongodb+srv://user:pass@cluster.mongodb.net/platform" \
    --storage-id "your-storage-uuid" \
    --storage-path /home/files \
    -n 2
```

### Options

| Option | Default | คำอธิบาย |
|---|---|---|
| `-n, --count, -w` | `1` | จำนวน worker instances |
| `--mongodb-uri` | `""` | MongoDB connection string |
| `--storage-id` | `""` | **จำเป็น** — UUID storage (ตรงกับ server-storage) |
| `--storage-path` | `/home/files` | Local storage path |
| `--port` | `8084` | Health check port |
| `--uninstall` | — | ถอนการติดตั้ง |

### Examples

```bash
# 2 workers บนเครื่อง storage เดียวกัน
curl -fsSL https://raw.githubusercontent.com/vdohide-core/server-transfer/main/install.sh | sudo -E bash -s -- \
    --mongodb-uri "mongodb+srv://..." \
    --storage-id "6c4de678-a29c-44c9-bc2d-9c281936a012" \
    --storage-path /home/files \
    -n 2

# Uninstall
curl -fsSL https://raw.githubusercontent.com/vdohide-core/server-transfer/main/install.sh | sudo bash -s -- --uninstall
```

### After install

```bash
# ดู logs ทุก worker
journalctl -u "server-transfer@*" -f

# ดู worker 1
journalctl -u "server-transfer@1" -f

# Restart ทุก worker
for i in $(seq 1 2); do systemctl restart server-transfer@$i; done
```

แต่ละ instance ได้ `WORKER_ID=transfer_{hostname}@{n}` อัตโนมัติจาก systemd (`server-transfer@1`, `@2`, ...)

---

## Configuration (.env)

```env
MONGODB_URI=mongodb+srv://user:password@host/database_name
STORAGE_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
STORAGE_PATH=/home/files
PORT=8084
LOG_PATH=logs/server-transfer.log
WORKER_ID=transfer_storage1@1
```

| Variable | Description | Default |
|---|---|---|
| `MONGODB_URI` | MongoDB connection string (รองรับ `MONGO_URI`, `DATABASE_URL`) | `mongodb://localhost:27017` |
| `STORAGE_ID` | **จำเป็น** — UUID ของ local storage node (ต้องตรงกับ `server-storage`) | — |
| `STORAGE_PATH` | path เก็บไฟล์บน disk | `/home/files` |
| `PORT` | Health check HTTP port | `8084` |
| `LOG_PATH` | Rotating log file path | `logs/server-transfer.log` |
| `WORKER_ID` | Worker identifier (ต้องไม่ซ้ำต่อ process) | `transfer_{hostname}@1` |

### เปิดใช้งาน worker

ใน MongoDB collection `settings`:

```json
{ "name": "transfer_enabled", "value": true }
```

---

## Worker Flow

### รับงาน

ไฟล์ที่ตรงเงื่อนไข:

- `status: ready_original`
- `type: video`
- ไม่ใช่ clone (`clonedFrom` ไม่มี)
- ไม่ถูก trash / delete
- มี ingest `processed` ที่ยังไม่ `deletedAt`

### ขั้นตอน (4 steps)

| Step | ทำอะไร |
|---|---|
| `download` | โหลดจาก S3 ลง temp dir |
| `extract` | แตก `sprite.zip` → `sprite/` |
| `install` | ย้ายไฟล์ไป `{STORAGE_PATH}/{fileId}/` |
| `media` | สร้าง media records + clone ไป `clonedFrom` |

### Skip logic (เช็ค medias ทั้งระบบ)

ก่อนโหลดแต่ละไฟล์:

```
medias.find({ fileId, resolution, deletedAt: null })
```

- **มี media แล้ว** (storage ไหนก็ได้) → ข้าม download, soft-delete ingest
- **ยังไม่มี** → โหลด → install → สร้าง media บน `STORAGE_ID` ของเครื่องนี้

### อัปเดต file status

ตั้ง `status: ready` เมื่อ `medias` มี `resolution: original` แล้ว (บน storage ใดก็ได้)

---

## S3 Object Layout

```
{fileId}/file_original.mp4
{fileId}/file_1080.mp4
{fileId}/file_720.mp4
{fileId}/file_480.mp4
{fileId}/file_360.mp4
{fileId}/sprite.zip
```

ปัจจุบัน `server-download` อัปโหลดแค่ `file_original.mp4` — ไฟล์อื่นจะมาจาก pipeline อื่นในอนาคต

---

## Local File Structure

```
{STORAGE_PATH}/
  └── {fileId}/
      ├── file_original.mp4
      ├── file_1080.mp4
      ├── file_720.mp4
      ├── file_480.mp4
      ├── file_360.mp4
      └── sprite/
          ├── sprite.vtt
          ├── 1.jpg
          └── ...
```

---

## Multi-Worker

รันได้หลาย worker เหมือน `server-download` — ใช้ systemd template `server-transfer@.service`:

```bash
# ติดตั้ง 3 workers
install.sh ... -n 3
# → server-transfer@1, @2, @3
# → WORKER_ID=transfer_hostname@1, transfer_hostname@2, transfer_hostname@3
```

| หัวข้อ | พฤติกรรม |
|---|---|
| ไฟล์เดียวกัน | **ไม่รับซ้ำ** — unique index `video_process.{fileId,processType}` |
| คนละไฟล์ | ทำงานพร้อมกันได้ |
| Health port | worker แรก bind `:8084` worker อื่นข้าม (worker loop ยังทำงาน) |

---

## API

### `GET /health`

```json
{
  "status": "ok",
  "service": "server-transfer",
  "worker": "storage1@1"
}
```

---

## Development

```bash
git clone https://github.com/vdohide-core/server-transfer.git
cd server-transfer

cp .env.example .env   # หรือสร้าง .env เอง

go run ./cmd
```

### Build

```bash
# Windows
build.bat

# Linux
GOOS=linux GOARCH=amd64 go build -o server-transfer ./cmd
```

---

## Database

### `video_process` (processType: `transfer`)

Timeline: `download` → `extract` → `install` → `media`

### `ingests` หลัง transfer

```json
{ "deletedAt": "2026-06-19T12:00:00Z" }
```

`server-service` จะลบ object บน S3 และลบ ingest record ตาม cleanup schedule

### `medias` ที่สร้าง

| type | resolution | fileName |
|---|---|---|
| `video` | `original` | `file_original.mp4` |
| `video` | `1080` / `720` / `480` / `360` | `file_{res}.mp4` |
| `thumbnail` | — | `sprite.vtt` |

Clone files (`clonedFrom`) ได้ media record ชี้ storage เดียวกัน โดย `server-storage` resolve path ผ่าน `file.clonedFrom`

---

## Related Services

| Service | Role |
|---|---|
| [server-download](https://github.com/vdohide-core/server-download) | ดาวน์โหลดวิดีโอ → อัป S3 → `ready_original` |
| **server-transfer** | S3 → local storage → medias |
| [server-transcode](https://github.com/vdohide-core/server-transcode) | Encode renditions + sprite บน storage |
| [server-storage](https://github.com/vdohide-core/server-storage) | เสิร์ฟไฟล์ static จาก `STORAGE_PATH` |
| [server-service](https://github.com/vdohide-core/server-service) | S3 cleanup หลัง soft-delete ingests |
