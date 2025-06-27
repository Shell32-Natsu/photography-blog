package image

import (
	"fmt"
	"os"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
)

type ExifInfo struct {
	Shutter  *string `json:"shutter,omitempty"`
	Model    *string `json:"model,omitempty"`
	Aperture *string `json:"aperture,omitempty"`
	Focal    *string `json:"focal,omitempty"`
	ISO      *string `json:"iso,omitempty"`
	Date     *string `json:"date,omitempty"`
	Parsed   *bool   `json:"parsed,omitempty"`
}

func IsLocalImage(path string) bool {
	return !strings.HasPrefix(path, "http")
}

func getExifField(x *exif.Exif, fieldName exif.FieldName) string {
	tag, err := x.Get(fieldName)
	if err != nil {
		return ""
	}
	
	switch fieldName {
	case exif.ExposureTime:
		num, denom, err := tag.Rat2(0)
		if err != nil {
			return ""
		}
		if denom == 0 {
			return ""
		}
		if num == 1 {
			return fmt.Sprintf("1/%d", denom)
		}
		return fmt.Sprintf("%d/%d", num, denom)
		
	case exif.FNumber:
		num, denom, err := tag.Rat2(0)
		if err != nil {
			return ""
		}
		if denom == 0 {
			return ""
		}
		fnum := float64(num) / float64(denom)
		return fmt.Sprintf("f/%.1f", fnum)
		
	case exif.ISOSpeedRatings:
		vals, err := tag.Int(0)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("%d", vals)
		
	case exif.FocalLengthIn35mmFilm:
		val, err := tag.Int(0)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("%d mm", val)
		
	case exif.Model:
		val, err := tag.StringVal()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(val)
		
	case exif.DateTimeDigitized:
		val, err := tag.StringVal()
		if err != nil {
			return ""
		}
		return val
		
	default:
		return ""
	}
}

func GetExifFromImage(imagePath string) *ExifInfo {
	path := strings.TrimPrefix(imagePath, "/")
	
	parsed := false
	ret := &ExifInfo{
		Shutter:  stringPtr(""),
		Model:    stringPtr(""),
		Aperture: stringPtr(""),
		Focal:    stringPtr(""),
		ISO:      stringPtr(""),
		Date:     stringPtr(""),
		Parsed:   &parsed,
	}
	
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not open file: %s\n", imagePath)
		return ret
	}
	defer file.Close()
	
	x, err := exif.Decode(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding EXIF: %v\n", err)
		return ret
	}
	
	parsed = true
	ret.Parsed = &parsed
	ret.Shutter = stringPtr(getExifField(x, exif.ExposureTime))
	ret.Model = stringPtr(getExifField(x, exif.Model))
	ret.Aperture = stringPtr(getExifField(x, exif.FNumber))
	ret.ISO = stringPtr(getExifField(x, exif.ISOSpeedRatings))
	ret.Focal = stringPtr(getExifField(x, exif.FocalLengthIn35mmFilm))
	ret.Date = stringPtr(getExifField(x, exif.DateTimeDigitized))
	
	return ret
}

func stringPtr(s string) *string {
	return &s
}