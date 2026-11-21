package api

import (
	"fmt"
	"path"
	"strings"
)

func printerUploadStorage(printerType, filename string) (string, error) {
	if filename == "" || strings.ContainsAny(filename, `/\`) || strings.Contains(filename, "..") {
		return "", fmt.Errorf("invalid filename")
	}
	ext := strings.ToLower(path.Ext(filename))
	switch printerType {
	case "prusalink":
		if ext != ".gcode" && ext != ".bgcode" {
			return "", fmt.Errorf("PrusaLink printers accept .gcode or .bgcode files")
		}
		// PrusaLink internal flash is read-only; removable media accepts writes.
		return "usb", nil
	case "snapmaker-u1":
		if ext != ".gcode" {
			return "", fmt.Errorf("U1/Klipper printers accept .gcode files only")
		}
		return "gcodes", nil
	default:
		return "", fmt.Errorf("select a PrusaLink or Klipper printer")
	}
}
