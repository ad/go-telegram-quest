package models

import (
	"fmt"
	"strings"
	"time"
)

type User struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
	IsBlocked bool
	CreatedAt time.Time
}

func (u *User) DisplayName() string {
	var parts []string
	if u.FirstName != "" {
		parts = append(parts, u.FirstName)
	}
	if u.LastName != "" {
		parts = append(parts, u.LastName)
	}
	if u.Username != "" {
		parts = append(parts, fmt.Sprintf("@%s", u.Username))
	}
	parts = append(parts, fmt.Sprintf("[%d]", u.ID))
	return strings.Join(parts, " ")
}
