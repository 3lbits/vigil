package devseed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/3lbits/vigil/internal/db"
	"github.com/google/uuid"
)

type stubUser struct {
	ProviderID string
	Email      string
	Name       string
	Role       string
}

var defaultStubUsers = []stubUser{
	{ProviderID: "dev-admin", Email: "dev-admin@localhost", Name: "Jordan Blake (Dev Admin)", Role: "admin"},
	{ProviderID: "dev-editor", Email: "dev-editor@localhost", Name: "Maya Chen (Dev Editor)", Role: "editor"},
	{ProviderID: "dev-editor-2", Email: "dev-editor-2@localhost", Name: "Liam Patel (Dev Editor)", Role: "editor"},
	{ProviderID: "dev-contributor", Email: "dev-contributor@localhost", Name: "Noah Rivera (Dev Contributor)", Role: "contributor"},
	{ProviderID: "dev-viewer", Email: "dev-viewer@localhost", Name: "Emma Rossi (Dev Viewer)", Role: "viewer"},
}

var (
	errNoOrganizations = errors.New("cannot assign users to organizations: no organizations")
	errMissingOrgNode  = errors.New("missing expected organization node")
)

func SeedStubUsers(ctx context.Context, q db.Querier) ([]db.User, error) {
	users := make([]db.User, 0, len(defaultStubUsers))
	for _, seed := range defaultStubUsers {
		user, err := q.UpsertDevStubUser(ctx, db.UpsertDevStubUserParams{
			ProviderID: seed.ProviderID,
			Email:      seed.Email,
			Name:       seed.Name,
			Role:       seed.Role,
		})
		if err != nil {
			return nil, fmt.Errorf("upsert dev stub user %s: %w", seed.ProviderID, err)
		}
		users = append(users, user)
	}
	return users, nil
}

type orgNode struct {
	Key       string
	Name      string
	ParentKey string
}

var orgTree = []orgNode{
	{Key: "dev-org-root", Name: "Northwind Digital"},
	{Key: "dev-org-security", Name: "Security & Compliance", ParentKey: "dev-org-root"},
	{Key: "dev-org-engineering", Name: "Engineering", ParentKey: "dev-org-root"},
	{Key: "dev-org-security-appsec", Name: "Application Security Team", ParentKey: "dev-org-security"},
	{Key: "dev-org-engineering-platform", Name: "Platform Reliability Team", ParentKey: "dev-org-engineering"},
}

func EnsureOrganizations(ctx context.Context, q db.Querier) ([]db.Organization, error) {
	orgs, err := q.ListOrganizations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}

	byKey := orgsByKey(orgs)
	if createErr := ensureOrgTree(ctx, q, byKey); createErr != nil {
		return nil, createErr
	}
	return collectOrgTree(byKey)
}

func AssignUsersToOrgs(ctx context.Context, q db.Querier, users []db.User, orgs []db.Organization) error {
	if len(orgs) == 0 {
		return errNoOrganizations
	}
	byKey := orgsByKey(orgs)
	roleKey := map[string]string{
		"admin":       "dev-org-root",
		"editor":      "dev-org-security",
		"contributor": "dev-org-security-appsec",
		"viewer":      "dev-org-engineering-platform",
	}
	for i, user := range users {
		org := orgs[i%len(orgs)]
		if key, ok := roleKey[user.Role]; ok {
			if mapped, exists := byKey[key]; exists {
				org = mapped
			}
		}
		_, err := q.SetUserOrg(ctx, db.SetUserOrgParams{
			ID:    user.ID,
			OrgID: uuid.NullUUID{UUID: org.ID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("set user org for %s: %w", user.ProviderID, err)
		}
	}
	return nil
}

func orgsByKey(orgs []db.Organization) map[string]db.Organization {
	byKey := make(map[string]db.Organization, len(orgs))
	for _, org := range orgs {
		if org.Key.Valid {
			byKey[org.Key.String] = org
		}
	}
	return byKey
}

func ensureOrgTree(ctx context.Context, q db.Querier, byKey map[string]db.Organization) error {
	for _, node := range orgTree {
		if _, ok := byKey[node.Key]; ok {
			continue
		}
		parentID := uuid.NullUUID{}
		if node.ParentKey != "" {
			parent, ok := byKey[node.ParentKey]
			if !ok {
				return fmt.Errorf("%w: parent %s for %s", errMissingOrgNode, node.ParentKey, node.Key)
			}
			parentID = uuid.NullUUID{UUID: parent.ID, Valid: true}
		}
		created, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
			Name:     node.Name,
			ParentID: parentID,
			Key:      sqlNullString(node.Key),
		})
		if err != nil {
			return fmt.Errorf("create organization %s: %w", node.Key, err)
		}
		byKey[node.Key] = created
	}
	return nil
}

func collectOrgTree(byKey map[string]db.Organization) ([]db.Organization, error) {
	out := make([]db.Organization, 0, len(orgTree))
	for _, node := range orgTree {
		org, ok := byKey[node.Key]
		if !ok {
			return nil, fmt.Errorf("%w: %s", errMissingOrgNode, node.Key)
		}
		out = append(out, org)
	}
	return out, nil
}

func sqlNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}
