package main

import (
	"log"
	"regexp"
)

// ── BlockExchange authors ────────────────────────────────────────────────────
//
// The BlockExchange library in class instances filters structures by the
// institute of their author. BlockExchange accounts are separate from game
// accounts, so the bridge reports which account a teacher signed in with
// ("blockexchange_linked"); teachers who never signed in are matched by their
// game name.

var blockExchangeUsername = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

func (c *controller) linkBlockExchangeAccount(player string, data map[string]interface{}) {
	username, _ := data["bx_username"].(string)
	if !blockExchangeUsername.MatchString(username) {
		return
	}
	if _, err := c.db.Exec("UPDATE teachers SET bx_username = ? WHERE username = ?", username, player); err != nil {
		log.Printf("[%s] failed to link BlockExchange account %s for %s: %v", pluginName, username, player, err)
	}
}

type blockExchangeInstitute struct {
	Name  string   `json:"name"`
	Users []string `json:"users"`
}

// blockExchangeInstitutes lists every institute with the BlockExchange
// usernames of its teachers, for the library's institute filter.
func (c *controller) blockExchangeInstitutes(enabled bool) []blockExchangeInstitute {
	result := []blockExchangeInstitute{}
	if !enabled {
		return result
	}
	rows, err := c.db.Query(`SELECT i.name, COALESCE(NULLIF(t.bx_username, ''), t.username)
		FROM institutes i LEFT JOIN teachers t ON t.institute_id = i.id
		ORDER BY i.name, t.username`)
	if err != nil {
		log.Printf("[%s] failed to list BlockExchange institutes: %v", pluginName, err)
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var user *string
		if err := rows.Scan(&name, &user); err != nil {
			log.Printf("[%s] failed to read BlockExchange institutes: %v", pluginName, err)
			return result
		}
		if len(result) == 0 || result[len(result)-1].Name != name {
			result = append(result, blockExchangeInstitute{Name: name, Users: []string{}})
		}
		if user != nil {
			last := &result[len(result)-1]
			last.Users = append(last.Users, *user)
		}
	}
	return result
}
