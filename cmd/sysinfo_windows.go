//go:build windows

package main

func getDiskUsage(path string) (total, used, free int64)  { return 0, 0, 0 }
func getMemoryUsage() (total, used int64)                  { return 0, 0 }
func getCPUPercent() float64                               { return 0 }
