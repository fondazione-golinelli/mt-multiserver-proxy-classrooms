package main

import (
	"bytes"
	"database/sql"
	"errors"
	"strings"
	"testing"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
	"github.com/HimbeerserverDE/srp"
)

type testAccountBackend struct {
	proxy.AuthBackend
	exists         bool
	err            error
	name           string
	salt, verifier []byte
	calls          int
}

func (a *testAccountBackend) Exists(string) bool { return a.exists }
func (a *testAccountBackend) CreateAccount(name string, salt, verifier []byte) error {
	a.calls++
	if a.err != nil {
		return a.err
	}
	a.name, a.salt, a.verifier = name, salt, verifier
	return nil
}
func TestAccountValidation(t *testing.T) {
	for _, name := range []string{"", "../student", "student/name", "student name", "singleplayer", strings.Repeat("x", 21)} {
		if validateAccountInput(name, "password123", "password123") == "" {
			t.Errorf("accepted invalid name %q", name)
		}
	}
	for _, password := range []string{"", "short", strings.Repeat("x", 257)} {
		if validateAccountInput("Student_1", password, password) == "" {
			t.Error("accepted invalid password length")
		}
	}
	if validateAccountInput("Student_1", "password123", "different") == "" {
		t.Fatal("accepted mismatched passwords")
	}
}
func TestAccountRegistrationRejectsExistingAndUnsupported(t *testing.T) {
	existing := &testAccountBackend{exists: true}
	if registerStudentAccount(existing, "Student", "password123", "password123") == "" || existing.calls != 0 {
		t.Fatal("attempted existing-account mutation")
	}
	if registerStudentAccount(nil, "Student", "password123", "password123") == "" {
		t.Fatal("accepted unavailable backend")
	}
	racing := &testAccountBackend{err: errors.New("duplicate")}
	if registerStudentAccount(racing, "Student", "password123", "password123") == "" {
		t.Fatal("ignored atomic creation failure")
	}
}
func TestCreatedAccountAuthenticatesWithLuantiIdentity(t *testing.T) {
	backend := &testAccountBackend{}
	const password = " password123 "
	if msg := registerStudentAccount(backend, "Student_1", password, password); msg != "" {
		t.Fatal(msg)
	}
	if backend.name != "Student_1" {
		t.Fatal("login name changed")
	}
	A, a, err := srp.InitiateHandshake()
	if err != nil {
		t.Fatal(err)
	}
	B, _, serverKey, err := srp.Handshake(A, backend.verifier)
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := srp.CompleteHandshake(A, a, []byte("student_1"), []byte(password), backend.salt, B)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(serverKey, clientKey) {
		t.Fatal("credentials incompatible with Luanti SRP")
	}
}

func TestAccountCreationRequiresClassManagement(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		"CREATE TABLE classes (id INTEGER PRIMARY KEY, name TEXT, created_by TEXT, created_at DATETIME)",
		"CREATE TABLE class_teachers (class_id INTEGER, username TEXT)",
		"CREATE TABLE class_assistants (class_id INTEGER, username TEXT)",
		"INSERT INTO classes VALUES (1, 'Class', 'owner', CURRENT_TIMESTAMP)",
		"INSERT INTO class_teachers VALUES (1, 'linked')",
		"INSERT INTO class_assistants VALUES (1, 'assistant')",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	c := &controller{db: db}
	for _, name := range []string{"owner", "linked"} {
		if !c.canManageClass(1, name) {
			t.Fatalf("teacher %s cannot manage class", name)
		}
	}
	if !c.canEditClassStudents(1, "assistant") {
		t.Fatal("assistant lost existing student management")
	}
	for _, name := range []string{"assistant", "stranger"} {
		if msg := c.createStudentAccount(1, name, "Student", "password123", "password123"); msg != T("Only class teachers and administrators can create accounts.") {
			t.Fatalf("unauthorized creation not rejected: %s", msg)
		}
	}
}

type testPasswordBackend struct {
	proxy.AuthBackend
	users          []proxy.User
	err            error
	salt, verifier []byte
	calls          int
}

func (a *testPasswordBackend) Exists(name string) bool {
	for _, u := range a.users {
		if u.Name == name {
			return true
		}
	}
	return false
}
func (a *testPasswordBackend) SetPasswd(_ string, salt, verifier []byte) error {
	a.calls++
	a.salt, a.verifier = salt, verifier
	return a.err
}
func (a *testPasswordBackend) Export() ([]proxy.User, error) { return a.users, nil }

func TestSetAccountPassword(t *testing.T) {
	backend := &testPasswordBackend{users: []proxy.User{{Name: "Student_1"}}}
	if setAccountPassword(backend, "Student_1", "short", "short") == "" || backend.calls != 0 {
		t.Fatal("accepted invalid password")
	}
	if setAccountPassword(backend, "Student_1", "password123", "different") == "" || backend.calls != 0 {
		t.Fatal("accepted mismatched passwords")
	}
	if setAccountPassword(backend, "Nobody", "password123", "password123") == "" || backend.calls != 0 {
		t.Fatal("created a missing account")
	}
	if msg := setAccountPassword(backend, "Student_1", "password123", "password123"); msg != "" || backend.calls != 1 {
		t.Fatalf("password not changed: %s", msg)
	}
	A, a, _ := srp.InitiateHandshake()
	B, _, serverKey, err := srp.Handshake(A, backend.verifier)
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := srp.CompleteHandshake(A, a, []byte("student_1"), []byte("password123"), backend.salt, B)
	if err != nil || !bytes.Equal(serverKey, clientKey) {
		t.Fatal("new password does not authenticate")
	}
	failing := &testPasswordBackend{users: backend.users, err: errors.New("disk full")}
	if setAccountPassword(failing, "Student_1", "password123", "password123") == "" {
		t.Fatal("ignored backend failure")
	}
}
