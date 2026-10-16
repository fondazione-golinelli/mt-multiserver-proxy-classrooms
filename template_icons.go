package main

import (
	"embed"
	"encoding/base64"
	"path"
	"strings"
)

// Map template icons for the template picker, drawn by tools/make_icons.py.
// They are sent inline as "[png:<base64>" textures, so neither the lobby nor
// the instance pool needs them as media.
//
//go:embed icons/*.png
var templateIconFiles embed.FS

var templateIcons = loadTemplateIcons()

func loadTemplateIcons() map[string]string {
	icons := make(map[string]string)
	entries, err := templateIconFiles.ReadDir("icons")
	if err != nil {
		return icons
	}
	for _, e := range entries {
		data, err := templateIconFiles.ReadFile(path.Join("icons", e.Name()))
		if err != nil {
			continue
		}
		icons[strings.TrimSuffix(e.Name(), ".png")] = "[png:" + base64.StdEncoding.EncodeToString(data)
	}
	return icons
}

// templateIcon is the picker icon of a template: the embedded icon named by
// its `icon` field or else by its key, or the globe.
func (c *controller) templateIcon(key string) string {
	name := key
	if tpl, ok := c.cfg.Templates[key]; ok && tpl.Icon != "" {
		name = tpl.Icon
	}
	if icon, ok := templateIcons[name]; ok {
		return icon
	}
	return iconGlobe
}
