package main

import (
	"fmt"
	"strings"
)

// ── UI kit shared by every classrooms formspec ─────────────────────────────
//
// Icons are textures built into the Luanti client (textures/base/pack), which
// are available in-game on every server without media from a pool. Proxy
// formspecs are not rewritten, so these plain names resolve on the client.

const (
	iconSearch   = "search.png"
	iconRefresh  = "refresh.png"
	iconPlus     = "plus.png"
	iconClose    = "clear.png"
	iconBack     = "prev_icon.png"
	iconNext     = "next_icon.png"
	iconEye      = "camera_btn.png"
	iconTeleport = "fly_btn.png"
	iconEnter    = "exit_btn.png"
	iconGear     = "debug_btn.png"
	iconCheck    = "checkbox_64.png"
	iconPlayer   = "player.png"
	iconPeople   = "server_view_clients.png"
	iconHeart    = "heart.png"
	iconPvP      = "server_flags_pvp.png"
	iconWarning  = "error_icon_orange.png"
	iconError    = "error_icon_red.png"
	iconGlobe    = "server_public.png"
	iconStar     = "server_favorite.png"
	iconGather   = "noclip_btn.png"
	// A "pause" symbol drawn with texture modifiers: two bars on a
	// transparent square.
	iconFreeze = "[fill:16x16:#00000000^[fill:4x12:3,2:#9fd8ff^[fill:4x12:9,2:#9fd8ff"
	// A padlock: shackle and body.
	iconLock = "[fill:16x16:#00000000^[fill:8x2:4,1:#e0b43f^[fill:2x6:4,1:#e0b43f^[fill:2x6:10,1:#e0b43f^[fill:12x8:2,7:#e0b43f^[fill:2x3:7,9:#4a3a12"
)

const (
	colorBg       = "#141a2a"
	colorCard     = "#202a44"
	colorRow      = "#28334f"
	colorButton   = "#34446a"
	colorPrimary  = "#2a8c7f"
	colorDanger   = "#9b2c3c"
	colorActive   = "#c7832a"
	colorTabIdle  = "#26304c"
	colorTabFocus = "#e94560"
	colorCurrent  = "#2d4a6b"
)

// fsOpen starts a formspec with the shared window style.
func fsOpen(b *strings.Builder, w, h float64) {
	b.WriteString("formspec_version[6]")
	b.WriteString(fmt.Sprintf("size[%g,%g]", w, h))
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", colorBg))
	// The game's formspec_prepend (e.g. Mineclonia's background9 and text
	// colors) is also prepended to proxy formspecs: paint over it.
	b.WriteString(box(0, 0, w, h, colorBg))
	b.WriteString(fmt.Sprintf("style_type[button,image_button;bgcolor=%s;border=false;textcolor=%s]", colorButton, light))
	b.WriteString(fmt.Sprintf("style_type[label,checkbox;textcolor=%s]", light))
	b.WriteString(fmt.Sprintf("style_type[field,pwdfield,textarea;textcolor=%s;border=true]", light))
	b.WriteString("style_type[label;font_size=*1]")
}

// fsHeader draws the title bar. back adds a Back button (btn_back), closeBtn
// adds a Close button (btn_close) on the right.
func fsHeader(b *strings.Builder, w float64, title, subtitle string, back, closeBtn bool) {
	b.WriteString(box(0, 0, w, 1.0, panel))
	x := 0.35
	if back {
		b.WriteString(iconBtn(0.2, 0.17, 0.66, "btn_back", iconBack, "Back"))
		x = 1.05
	}
	b.WriteString(fmt.Sprintf("label[%g,0.38;%s]", x, fmtEsc(mcColorize(light, title))))
	if subtitle != "" {
		b.WriteString(fmt.Sprintf("label[%g,0.75;%s]", x, fmtEsc(mcColorize(muted, subtitle))))
	}
	if closeBtn {
		b.WriteString(fmt.Sprintf("image_button_exit[%g,0.17;0.66,0.66;%s;btn_close;]", w-0.86, iconClose))
		b.WriteString("tooltip[btn_close;Close]")
	}
	b.WriteString(box(0, 1.0, w, 0.05, accent))
}

// tooltip escapes the text of a hover tooltip.
func tooltip(name, text string) string {
	return fmt.Sprintf("tooltip[%s;%s]", fmtEsc(name), fmtEsc(text))
}

// escapeTexMod escapes a texture string for use inside [combine.
func escapeTexMod(t string) string {
	t = strings.ReplaceAll(t, "\\", "\\\\")
	t = strings.ReplaceAll(t, "^", "\\^")
	return strings.ReplaceAll(t, ":", "\\:")
}

// fitIcon returns a texture with the aspect ratio of a w x h button and the
// icon centered in it. image_button stretches its texture over the whole
// button, which distorts square icons on wide buttons.
func fitIcon(icon string, w, h float64) string {
	const size, iconSize = 128, 96
	width := int(float64(size)*w/h + 0.5)
	if width < size {
		width = size
	}
	return fmt.Sprintf("[combine:%dx%d:%d,%d=%s", width, size, (width-iconSize)/2, (size-iconSize)/2,
		escapeTexMod(icon+"^[resize:96x96"))
}

// iconBtn is a square icon button with a hover tooltip.
func iconBtn(x, y, size float64, name, icon, tooltip string) string {
	s := fmt.Sprintf("image_button[%g,%g;%g,%g;%s;%s;]", x, y, size, size, fmtEsc(icon), fmtEsc(name))
	if tooltip != "" {
		s += fmt.Sprintf("tooltip[%s;%s]", fmtEsc(name), fmtEsc(tooltip))
	}
	return s
}

// styledBtn is a text button with its own background color.
func styledBtn(x, y, w, h float64, name, label, color string) string {
	return fmt.Sprintf("style[%s;bgcolor=%s]", fmtEsc(name), color) + btn(x, y, w, h, fmtEsc(name), label)
}

// actionTile is a big icon button with a caption underneath. active
// highlights toggles that are currently on.
func actionTile(b *strings.Builder, x, y, w float64, name, icon, caption, tooltip string, active bool) {
	color := colorButton
	if active {
		color = colorActive
	}
	b.WriteString(fmt.Sprintf("style[%s;bgcolor=%s]", name, color))
	b.WriteString(fmt.Sprintf("image_button[%g,%g;%g,1.15;%s;%s;]", x, y, w, fmtEsc(fitIcon(icon, w, 1.15)), name))
	if tooltip != "" {
		b.WriteString(fmt.Sprintf("tooltip[%s;%s]", name, fmtEsc(tooltip)))
	}
	textColor := light
	if active {
		textColor = warning
	}
	b.WriteString(fmt.Sprintf("hypertext[%g,%g;%g,0.5;%s_caption;<global background=none margin=0><center><style color=%s>%s</style></center>]",
		x, y+1.2, w, name, textColor, hyperEsc(caption)))
}

// hyperEsc escapes text for use inside hypertext[] markup.
func hyperEsc(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "<", "\\<")
	s = strings.ReplaceAll(s, ">", "\\>")
	return fmtEsc(s)
}

// statusDot draws a small colored square used as an online/running marker.
func statusDot(x, y float64, color string) string {
	return box(x, y, 0.22, 0.22, color)
}

// sectionTitle draws an uppercase section heading.
func sectionTitle(x, y float64, text string) string {
	return coloredLbl(x, y, muted, strings.ToUpper(text))
}

// hint draws a muted help line.
func hint(x, y float64, text string) string {
	return coloredLbl(x, y, muted, text)
}

// tabBar draws tab buttons; the active one is highlighted. tabs is a list of
// {fieldName, label} pairs.
func tabBar(b *strings.Builder, x, y, tabW float64, active string, tabs [][2]string) {
	for i, t := range tabs {
		color := colorTabIdle
		if t[0] == active {
			color = colorTabFocus
		}
		b.WriteString(styledBtn(x+float64(i)*(tabW+0.1), y, tabW, 0.62, t[0], t[1], color))
	}
}

func instanceStatusColor(status string) string {
	switch status {
	case "running":
		return success
	case "provisioning":
		return warning
	case "stopped":
		return danger
	}
	return muted
}

func instanceStatusLabel(status string) string {
	switch status {
	case "running":
		return "Online"
	case "provisioning":
		return "Creating…"
	case "stopped":
		return "Stopped"
	}
	return status
}
