package main

import (
	"fmt"
	"strings"
)

// textDomain is the translation domain of this plugin's player-facing text.
// The translations are shipped by the classrooms_bridge mod as
// locale/classrooms_proxy.<lang>.tr, because clients only load translation
// files from the media of the server they are on.
const textDomain = "classrooms_proxy"

// T marks English text for translation by the player's client, like S() in a
// Luanti mod. Arguments replace @1, @2, ... which must appear in ascending
// order. The result carries escape sequences: do not measure, cut or change
// the case of it, and do not use it in logs or mod channel messages.
func T(text string, args ...any) string {
	var b strings.Builder
	b.WriteString("\x1b(T@" + textDomain + ")")
	for i := 0; i < len(text); i++ {
		if text[i] == '@' && i+1 < len(text) && text[i+1] >= '1' && text[i+1] <= '9' {
			if n := int(text[i+1] - '1'); n < len(args) {
				b.WriteString("\x1bF" + fmt.Sprint(args[n]) + "\x1bE")
				i++
				continue
			}
		}
		b.WriteByte(text[i])
	}
	b.WriteString("\x1bE")
	return b.String()
}
