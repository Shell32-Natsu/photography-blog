package parser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Website struct {
	Title       *string      `json:"title,omitempty"`
	Description *string      `json:"description,omitempty"`
	Children    []Website    `json:"children,omitempty"`
	URL         *string      `json:"url,omitempty"`
	Category    *string      `json:"category,omitempty"`
	Author      *string      `json:"author,omitempty"`
	Path        *string      `json:"path,omitempty"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs,omitempty"`
	Exif        *ExifInfo    `json:"exif,omitempty"`
	Extra       *Extra       `json:"extra,omitempty"`
}

type Breadcrumb struct {
	Title   string `json:"title"`
	Path    string `json:"path"`
	Current bool   `json:"current"`
}

type Extra struct {
	ImageExifQuerySuffix string `json:"image_exif_query_suffix"`
	ImageStyleSuffix     string `json:"image_style_suffix"`
}

type ExifInfo struct {
	Shutter  *string `json:"shutter,omitempty"`
	Model    *string `json:"model,omitempty"`
	Aperture *string `json:"aperture,omitempty"`
	Focal    *string `json:"focal,omitempty"`
	ISO      *string `json:"iso,omitempty"`
	Date     *string `json:"date,omitempty"`
	Parsed   *bool   `json:"parsed,omitempty"`
}

func CreateBreadcrumbs(itemPath string) []Breadcrumb {
	decoded, err := url.QueryUnescape(itemPath)
	if err != nil {
		decoded = itemPath
	}
	
	titleList := strings.Split(decoded, "/")
	pathList := strings.Split(itemPath, "/")
	breadcrumbs := []Breadcrumb{}
	
	for i, title := range titleList {
		if title != "" {
			path := strings.Join(pathList[0:i+1], "/")
			breadcrumbs = append(breadcrumbs, Breadcrumb{
				Title:   title,
				Path:    path,
				Current: false,
			})
		}
	}
	
	if len(breadcrumbs) > 0 {
		breadcrumbs[len(breadcrumbs)-1].Current = true
	}
	
	return breadcrumbs
}

func GetCurrentPath() (string, error) {
	return os.Getwd()
}

func GetConfigPath() (string, error) {
	currentPath, err := GetCurrentPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(currentPath, "config.json"), nil
}

func ReadConfig() (string, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return "", fmt.Errorf("could not get config path: %w", err)
	}
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("could not read config file: %w", err)
	}
	
	return string(data), nil
}

func Parse() (*Website, error) {
	configStr, err := ReadConfig()
	if err != nil {
		return nil, err
	}
	
	var config Website
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		return nil, fmt.Errorf("could not parse config file: %w", err)
	}
	
	return &config, nil
}