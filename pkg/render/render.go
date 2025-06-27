package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/flosch/pongo2/v6"
	"github.com/photography-blog/pkg/parser"
)

func GetPathOfFile(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, "/")
}

func CreateDirectories(pathStr string) error {
	return os.MkdirAll(pathStr, 0755)
}

func CreateFile(pathStr string, content []byte) error {
	filePath := GetPathOfFile(pathStr)
	if err := CreateDirectories(filePath); err != nil {
		return fmt.Errorf("could not create directory %s: %w", filePath, err)
	}
	
	if err := os.WriteFile(pathStr, content, 0644); err != nil {
		return fmt.Errorf("could not write to file %s: %w", pathStr, err)
	}
	
	return nil
}

type Renderer struct {
	templateSet *pongo2.TemplateSet
	list        map[string]*parser.Website
}

func NewRenderer() (*Renderer, error) {
	loader := pongo2.MustNewLocalFileSystemLoader("template/")
	templateSet := pongo2.NewSet("photoblog", loader)
	
	return &Renderer{
		templateSet: templateSet,
		list:        make(map[string]*parser.Website),
	}, nil
}

func (r *Renderer) Cache(pathStr string, data *parser.Website) {
	r.list[pathStr] = data
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *Renderer) Render(path string, data *parser.Website) error {
	template, err := r.templateSet.FromFile("index.html")
	if err != nil {
		return fmt.Errorf("could not load template for %s: %w", path, err)
	}
	
	// Convert children to map format for template
	var children []map[string]interface{}
	if data.Children != nil {
		for _, child := range data.Children {
			// Dereference pointers for template access
			childMap := map[string]interface{}{
				"title":       derefString(child.Title),
				"description": derefString(child.Description),
				"url":         derefString(child.URL),
				"category":    derefString(child.Category),
				"path":        derefString(child.Path),
				"children":    child.Children,
				"exif":        child.Exif,
			}
			children = append(children, childMap)
		}
	}
	
	ctx := pongo2.Context{
		"title":       data.Title,
		"description": data.Description,
		"children":    children,
		"url":         data.URL,
		"category":    data.Category,
		"author":      data.Author,
		"path":        data.Path,
		"breadcrumbs": data.Breadcrumbs,
		"exif":        data.Exif,
		"extra":       data.Extra,
	}
	
	rendered, err := template.Execute(ctx)
	if err != nil {
		return fmt.Errorf("could not render template for %s: %w", path, err)
	}
	
	if err := CreateFile(path, []byte(rendered)); err != nil {
		return err
	}
	
	return nil
}

func (r *Renderer) RenderAll() error {
	for path, data := range r.list {
		if err := r.Render(path, data); err != nil {
			return err
		}
	}
	return nil
}