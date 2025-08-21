package controller

const (
	appliedAnnoKey = "labels.shahaf.com/applied" // JSON of map[string]string
	FinalizerName  = "labels.shahaf.com/finalizer"
	StandardCRName = "labels" // Standard name for NamespaceLabel CRs (singleton pattern)
)

// ProtectionResult represents the result of applying protection logic
type ProtectionResult struct {
	AllowedLabels    map[string]string
	ProtectedSkipped []string
	Warnings         []string
	ShouldFail       bool
}
