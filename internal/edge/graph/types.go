package graph

// RelationType defines the type of relationship between resources
type RelationType string

const (
	// Structural composition relationships (drive shape identity)
	Owns                RelationType = "Owns"
	UsesServiceAccount  RelationType = "UsesServiceAccount"
	SelectsWorkload     RelationType = "SelectsWorkload"
	BindsSubject        RelationType = "BindsSubject"
	GrantsRole          RelationType = "GrantsRole"
	ClaimsStorage       RelationType = "ClaimsStorage"
	UsesHeadlessService RelationType = "UsesHeadlessService"
	Mounts              RelationType = "Mounts"
	References          RelationType = "References"

	// Provenance/context relationships (boundary and ownership, not composition)
	ManagedBy        RelationType = "ManagedBy"
	BelongsToRelease RelationType = "BelongsToRelease"

	// Logical grouping relationships (contextual membership)
	MemberOf        RelationType = "MemberOf"
	MemberOfRelease RelationType = "MemberOfRelease"

	// Authority handoff relationships (control resource → governed resources)
	Reconciles RelationType = "Reconciles" // Application → Group members (active lifecycle authority)
	Generates  RelationType = "Generates"  // ApplicationSet → Application (upstream generator)
	Provisions RelationType = "Provisions" // External authority → control resource (how it arrived)
)

// RelationshipLayer classifies relationships for fingerprinting purposes
type RelationshipLayer string

const (
	// LayerDefining — relationships that distinguish what a deployment IS
	LayerDefining RelationshipLayer = "defining"

	// LayerFramework — generated controller machinery (excluded from semantic fingerprint)
	LayerFramework RelationshipLayer = "framework"

	// LayerProvenance — ownership/release membership (boundary, not composition)
	LayerProvenance RelationshipLayer = "provenance"
)

// ClassifyRelationship returns the fingerprint layer for a relationship type
func ClassifyRelationship(relType RelationType) RelationshipLayer {
	switch relType {
	case UsesServiceAccount, SelectsWorkload, BindsSubject, GrantsRole,
		ClaimsStorage, UsesHeadlessService, Mounts, References:
		return LayerDefining
	case ManagedBy, BelongsToRelease, MemberOf, MemberOfRelease,
		Reconciles, Generates, Provisions:
		return LayerProvenance
	case Owns:
		// Owns is framework when it's controller→generated (Deployment→ReplicaSet)
		// but defining when it's explicit (e.g., CRD→owned resource)
		// Caller must use IsFrameworkOwnership for further classification
		return LayerFramework
	default:
		return LayerDefining
	}
}

// Edge represents a directed relationship between two resources
type Edge struct {
	Source     string       // source resource key
	Target     string       // target resource key
	Type       RelationType // relationship type
	Evidence   string       // what field established this relationship
	Confidence string       // ExplicitField, SelectorMatch, LabelAssociation, NamingConvention
}
