package models

func (f *File) IsTrashed() bool {
	return f.Metadata != nil && f.Metadata.TrashedAt != nil
}

func (f *File) IsDeleted() bool {
	return f.Metadata != nil && f.Metadata.DeletedAt != nil
}
