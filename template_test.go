package main

import (
	"strings"
	"testing"
)

func TestValidateTemplateEggLimits(t *testing.T) {
	ok := templateConfig{TemplateName: "moon", ServerDescription: strings.Repeat("é", 64)}
	if err := validateTemplate("moon", &ok); err != nil {
		t.Fatalf("64 characters must pass: %v", err)
	}

	long := templateConfig{TemplateName: "moon", ServerDescription: strings.Repeat("a", 65)}
	err := validateTemplate("moon", &long)
	if err == nil || !strings.Contains(err.Error(), "server_description") {
		t.Fatalf("65-character description must fail, got %v", err)
	}

	prefix := templateConfig{TemplateName: "x", NamePrefix: strings.Repeat("p", 21)}
	if err := validateTemplate("x", &prefix); err == nil {
		t.Fatal("name_prefix over 20 characters must fail")
	}

	// The key becomes the default name prefix.
	key := templateConfig{TemplateName: "x"}
	if err := validateTemplate(strings.Repeat("k", 21), &key); err == nil {
		t.Fatal("a template key over 20 characters without name_prefix must fail")
	}
}

func TestTemplateIcons(t *testing.T) {
	for _, name := range []string{"italy", "spain", "hungary", "germany", "poland", "flat", "generic", "moon", "mars"} {
		icon, ok := templateIcons[name]
		if !ok || !strings.HasPrefix(icon, "[png:") {
			t.Errorf("icon %q missing", name)
		}
		// Characters that would break the formspec element or the texture string.
		if strings.ContainsAny(icon[1:], "[];,\\^") || strings.Count(icon, ":") != 1 {
			t.Errorf("icon %q is not formspec-safe", name)
		}
	}
	c := &controller{cfg: classroomsConfig{Templates: map[string]templateConfig{
		"moon":  {},
		"other": {},
		"alias": {Icon: "mars"},
	}}}
	if c.templateIcon("moon") != templateIcons["moon"] || c.templateIcon("alias") != templateIcons["mars"] {
		t.Error("icon by key or by icon field not found")
	}
	if c.templateIcon("other") != iconGlobe {
		t.Error("unknown template must fall back to the globe")
	}
}

func TestRetiredTemplatesNotListed(t *testing.T) {
	c := &controller{cfg: classroomsConfig{Templates: map[string]templateConfig{
		"moon": {Public: true},
		"hub":  {Public: true, Retired: true},
		"test": {Retired: true},
		"wip":  {},
	}}}
	if got := strings.Join(c.getPublicTemplates(), ","); got != "moon" {
		t.Errorf("public templates = %s", got)
	}
	if got := strings.Join(c.getAllTemplates(), ","); got != "moon,wip" {
		t.Errorf("admin templates = %s", got)
	}
	if _, err := c.provisionInstance(nil, "x", "hub", "y"); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Errorf("provisioning a retired template must fail, got %v", err)
	}
}
