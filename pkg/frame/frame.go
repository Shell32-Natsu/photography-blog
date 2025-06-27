package frame

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/photography-blog/pkg/config"
	"github.com/photography-blog/pkg/parser"
	"github.com/photography-blog/pkg/render"
)

func WalkList(
	item *parser.Website,
	pathPrefix string,
	renderer *render.Renderer,
	depth uint8,
	extra parser.Extra,
) {
	path := pathPrefix

	if depth != 0 {
		if item.Title != nil {
			path = fmt.Sprintf("%s/%s", path, *item.Title)
		}
	}

	if item.Extra != nil {
		extra = *item.Extra
	} else {
		item.Extra = &extra
	}

	if item.Exif != nil && item.Exif.Parsed != nil {
		parsed := true
		item.Exif.Parsed = &parsed
	} else {
		if item.URL != nil && isLocalImage(*item.URL) {
			exif := getExifFromImage(*item.URL)
			item.Exif = exif
		}
	}

	if item.Children != nil && len(item.Children) > 0 {
		depth++

		// Encode path parts
		pathParts := strings.Split(path, "/")
		encodedParts := make([]string, len(pathParts))
		for i, part := range pathParts {
			encodedParts[i] = url.QueryEscape(part)
		}
		itemPath := strings.Join(encodedParts, "/")
		itemPath = strings.Replace(itemPath, config.PublicPath, "", 1)

		indexFilePath := fmt.Sprintf("%s/index.html", path)

		// Process children and set their paths
		for i := range item.Children {
			child := &item.Children[i]
			
			// Set child path and category for template rendering
			if child.Title != nil {
				var childPath string
				if itemPath == "" {
					childPath = fmt.Sprintf("/%s", url.QueryEscape(*child.Title))
				} else {
					childPath = fmt.Sprintf("%s/%s", itemPath, url.QueryEscape(*child.Title))
				}
				child.Path = &childPath
				
				// Determine category based on whether it has children
				if child.Children != nil && len(child.Children) > 0 {
					category := "album"
					child.Category = &category
				} else {
					category := "photo"
					child.Category = &category
				}
			}
			
			WalkList(child, path, renderer, depth, extra)
		}

		// Set breadcrumbs and metadata
		item.Breadcrumbs = parser.CreateBreadcrumbs(itemPath)
		item.Path = &itemPath
		category := "album"
		item.Category = &category

		renderer.Cache(indexFilePath, item)
	}
}

func CreatePathFromConfig(cfg *parser.Website) error {
	renderer, err := render.NewRenderer()
	if err != nil {
		return fmt.Errorf("could not initialize renderer: %w", err)
	}

	extra := parser.Extra{
		ImageExifQuerySuffix: "",
		ImageStyleSuffix:     "",
	}

	WalkList(cfg, config.PublicPath, renderer, 0, extra)
	
	if err := renderer.RenderAll(); err != nil {
		return fmt.Errorf("could not render all templates: %w", err)
	}

	return nil
}

func isLocalImage(path string) bool {
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

func getExifFromImage(imagePath string) *parser.ExifInfo {
	path := strings.TrimPrefix(imagePath, "/")
	
	parsed := false
	ret := &parser.ExifInfo{
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