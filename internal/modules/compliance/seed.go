package compliance

import (
	"context"
	"fmt"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/modregistry"
)

type frameworkSeed struct {
	Name        string
	ShortName   string
	Version     string
	Description string
	Reqs        []requirementSeed
}

type requirementSeed struct {
	Ref   string
	Title string
}

var frameworkSeeds = []frameworkSeed{
	{
		Name:        "ISO/IEC 27001",
		ShortName:   "ISO27001",
		Version:     "2022",
		Description: "Information security management system baseline.",
		Reqs: []requirementSeed{
			{Ref: "A.5.15", Title: "Access control policy"},
			{Ref: "A.5.16", Title: "Identity management"},
			{Ref: "A.8.9", Title: "Configuration management"},
			{Ref: "A.8.8", Title: "Management of technical vulnerabilities"},
			{Ref: "A.5.24", Title: "Information security incident planning"},
		},
	},
	{
		Name:        "NIST Cybersecurity Framework",
		ShortName:   "NIST-CSF",
		Version:     "2.0",
		Description: "Cybersecurity outcomes across Govern, Identify, Protect, Detect, Respond, Recover.",
		Reqs: []requirementSeed{
			{Ref: "GV.RM-01", Title: "Risk management strategy is established"},
			{Ref: "ID.AM-01", Title: "Inventory of assets is maintained"},
			{Ref: "PR.AA-01", Title: "Identities and credentials are managed"},
			{Ref: "DE.CM-01", Title: "Continuous monitoring is performed"},
			{Ref: "RS.MA-01", Title: "Incident response actions are coordinated"},
		},
	},
	{
		Name:        "CIS Controls",
		ShortName:   "CIS-V8",
		Version:     "8.0",
		Description: "Prioritized safeguards for enterprise cyber defense.",
		Reqs: []requirementSeed{
			{Ref: "CIS-01", Title: "Inventory and control of enterprise assets"},
			{Ref: "CIS-04", Title: "Secure configuration of enterprise assets and software"},
			{Ref: "CIS-05", Title: "Account management"},
			{Ref: "CIS-06", Title: "Access control management"},
			{Ref: "CIS-17", Title: "Incident response management"},
		},
	},
	{
		Name:        "SOC 2 Trust Services Criteria",
		ShortName:   "SOC2-TSC",
		Version:     "2017",
		Description: "Security and availability trust criteria baseline.",
		Reqs: []requirementSeed{
			{Ref: "CC6.1", Title: "Logical and physical access controls"},
			{Ref: "CC6.2", Title: "User authentication and authorization"},
			{Ref: "CC7.1", Title: "System operation monitoring"},
			{Ref: "CC7.2", Title: "Change management and anomaly detection"},
			{Ref: "CC7.3", Title: "Incident response and remediation"},
		},
	},
	{
		Name:        "PCI DSS",
		ShortName:   "PCI-DSS",
		Version:     "4.0",
		Description: "Payment card data protection baseline.",
		Reqs: []requirementSeed{
			{Ref: "PCI-01", Title: "Install and maintain network security controls"},
			{Ref: "PCI-03", Title: "Protect stored account data"},
			{Ref: "PCI-06", Title: "Develop and maintain secure systems"},
			{Ref: "PCI-07", Title: "Restrict access to system components and cardholder data"},
			{Ref: "PCI-10", Title: "Log and monitor all access to system components"},
		},
	},
	{
		Name:        "COSO Internal Control Framework",
		ShortName:   "COSO-IC",
		Version:     "2013",
		Description: "Enterprise internal control components for governance and financial reporting.",
		Reqs: []requirementSeed{
			{Ref: "CE-01", Title: "Control environment responsibilities are defined"},
			{Ref: "RA-01", Title: "Enterprise risks are identified and analyzed"},
			{Ref: "CA-01", Title: "Control activities are selected and documented"},
			{Ref: "IC-01", Title: "Information and communication supports control execution"},
			{Ref: "MA-01", Title: "Ongoing monitoring evaluates control effectiveness"},
		},
	},
	{
		Name:        "ISO 22301",
		ShortName:   "ISO22301",
		Version:     "2019",
		Description: "Business continuity management system requirements.",
		Reqs: []requirementSeed{
			{Ref: "BC-4.1", Title: "Context and continuity requirements are established"},
			{Ref: "BC-6.1", Title: "Continuity objectives and planning actions are defined"},
			{Ref: "BC-8.4", Title: "Business continuity plans and procedures are maintained"},
			{Ref: "BC-8.5", Title: "Exercise program validates continuity capability"},
			{Ref: "BC-10.1", Title: "Nonconformities and corrective actions are tracked"},
		},
	},
}

func (complianceModule) DevSeed(ctx context.Context, deps modregistry.Dependencies) error {
	frameworks, err := deps.Queries.ListFrameworks(ctx)
	if err != nil {
		return fmt.Errorf("list frameworks: %w", err)
	}

	seedFrameworks := make([]db.Framework, 0, len(frameworkSeeds))
	for _, seed := range frameworkSeeds {
		framework, ensureErr := ensureFramework(ctx, deps.Queries, frameworks, seed)
		if ensureErr != nil {
			return ensureErr
		}
		seedFrameworks = append(seedFrameworks, framework)
	}

	for i, framework := range seedFrameworks {
		if err := ensureFrameworkRequirements(ctx, deps.Queries, framework, frameworkSeeds[i].Reqs); err != nil {
			return err
		}
	}
	return nil
}

func ensureFramework(ctx context.Context, q db.Querier, frameworks []db.Framework, seed frameworkSeed) (db.Framework, error) {
	for _, framework := range frameworks {
		if framework.ShortName == seed.ShortName {
			return framework, nil
		}
	}
	created, err := q.CreateFramework(ctx, db.CreateFrameworkParams{
		Name:          seed.Name,
		ShortName:     seed.ShortName,
		Version:       seed.Version,
		Description:   seed.Description,
		FrameworkType: "regulation",
	})
	if err != nil {
		return db.Framework{}, fmt.Errorf("create framework %s: %w", seed.ShortName, err)
	}
	return created, nil
}

func ensureFrameworkRequirements(ctx context.Context, q db.Querier, framework db.Framework, reqs []requirementSeed) error {
	requirements, err := q.ListRequirementsByFramework(ctx, framework.ID)
	if err != nil {
		return fmt.Errorf("list requirements for framework %s: %w", framework.ShortName, err)
	}
	existingByRef := make(map[string]bool, len(requirements))
	for _, req := range requirements {
		existingByRef[req.Ref] = true
	}
	for i, reqSeed := range reqs {
		if existingByRef[reqSeed.Ref] {
			continue
		}
		if _, createErr := q.CreateRequirement(ctx, db.CreateRequirementParams{
			FrameworkID: framework.ID,
			Ref:         reqSeed.Ref,
			Title:       reqSeed.Title,
			Description: "Baseline requirement seeded for realistic developer workflows.",
			SortOrder:   int32(i + 1),
		}); createErr != nil {
			return fmt.Errorf("create requirement %s: %w", reqSeed.Ref, createErr)
		}
	}
	return nil
}
