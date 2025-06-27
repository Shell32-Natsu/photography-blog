package config

import "runtime"

const (
	TemplateFilePath = "template/*.html"
	PublicPath       = "public"
	AssetsPath       = "template/assets"
	ImagePath        = "image"
)

func GetPublicAssetPath() string {
	if runtime.GOOS == "windows" {
		return `public\assets\`
	}
	return PublicPath + "/assets"
}

func GetAssetsPath() string {
	if runtime.GOOS == "windows" {
		return `template\assets`
	}
	return AssetsPath
}