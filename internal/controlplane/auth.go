package controlplane

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	PermissionMCPApprove      = "mcp.approve"
	PermissionPolicyRead      = "policy.read"
	PermissionPolicyReload    = "policy.reload"
	PermissionWorkspaceCreate = "workspace.create"
	PermissionWorkspaceRead   = "workspace.read"
	PermissionUsageWrite      = "usage.write"
	PermissionUsageRead       = "usage.read"
	PermissionLLMKeyCreate    = "llm.key.create"
	PermissionLLMKeyRead      = "llm.key.read"
	PermissionLLMKeyRotate    = "llm.key.rotate"
	PermissionLLMKeyRevoke    = "llm.key.revoke"
)

type Principal struct {
	ID          string   `json:"id"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

type UsersFile struct {
	Version int             `yaml:"version" json:"version"`
	Roles   map[string]Role `yaml:"roles" json:"roles,omitempty"`
	Users   []User          `yaml:"users" json:"users"`
}

type Role struct {
	Permissions []string `yaml:"permissions" json:"permissions"`
}

type User struct {
	ID          string   `yaml:"id" json:"id"`
	Roles       []string `yaml:"roles" json:"roles,omitempty"`
	Permissions []string `yaml:"permissions" json:"permissions,omitempty"`
	TokenSHA256 string   `yaml:"token_sha256" json:"token_sha256"`
}

type Authenticator struct {
	users map[string]User
	roles map[string]Role
}

func LoadAuthenticator(path string) (Authenticator, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Authenticator{}, err
	}
	var doc UsersFile
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Authenticator{}, err
	}
	if err := doc.Validate(); err != nil {
		return Authenticator{}, err
	}
	auth := Authenticator{
		users: map[string]User{},
		roles: map[string]Role{},
	}
	for key, role := range doc.Roles {
		auth.roles[strings.TrimSpace(key)] = role
	}
	for _, user := range doc.Users {
		auth.users[user.ID] = user
	}
	return auth, nil
}

func (d UsersFile) Validate() error {
	var problems []string
	if d.Version <= 0 {
		problems = append(problems, "version must be positive")
	}
	if len(d.Users) == 0 {
		problems = append(problems, "users must not be empty")
	}
	seen := map[string]bool{}
	for index, user := range d.Users {
		location := fmt.Sprintf("users[%d]", index)
		if strings.TrimSpace(user.ID) == "" {
			problems = append(problems, location+".id is required")
		}
		if seen[user.ID] {
			problems = append(problems, fmt.Sprintf("duplicate user id %q", user.ID))
		}
		seen[user.ID] = true
		if strings.TrimSpace(user.TokenSHA256) == "" {
			problems = append(problems, location+".token_sha256 is required")
		}
		if _, err := hex.DecodeString(strings.TrimSpace(user.TokenSHA256)); err != nil {
			problems = append(problems, location+".token_sha256 must be hex")
		}
		for _, role := range user.Roles {
			if _, ok := d.Roles[role]; !ok {
				problems = append(problems, fmt.Sprintf("%s references unknown role %q", location, role))
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func TokenSHA256(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (a Authenticator) AuthenticateBearer(header string) (Principal, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return Principal{}, errors.New("missing bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return Principal{}, errors.New("empty bearer token")
	}
	tokenHash := TokenSHA256(token)
	for _, user := range a.users {
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(user.TokenSHA256)), []byte(tokenHash)) != 1 {
			continue
		}
		return a.principalFor(user), nil
	}
	return Principal{}, errors.New("invalid bearer token")
}

func (a Authenticator) principalFor(user User) Principal {
	permissions := map[string]bool{}
	for _, permission := range user.Permissions {
		permission = strings.TrimSpace(permission)
		if permission != "" {
			permissions[permission] = true
		}
	}
	for _, roleName := range user.Roles {
		role, ok := a.roles[roleName]
		if !ok {
			continue
		}
		for _, permission := range role.Permissions {
			permission = strings.TrimSpace(permission)
			if permission != "" {
				permissions[permission] = true
			}
		}
	}
	out := Principal{
		ID:    user.ID,
		Roles: append([]string(nil), user.Roles...),
	}
	for permission := range permissions {
		out.Permissions = append(out.Permissions, permission)
	}
	sort.Strings(out.Roles)
	sort.Strings(out.Permissions)
	return out
}

func (p Principal) HasPermission(permission string) bool {
	for _, item := range p.Permissions {
		if item == permission {
			return true
		}
	}
	return false
}
