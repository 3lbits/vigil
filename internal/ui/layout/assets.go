package layout

// AssetVer is set at startup from a content hash of the embedded static files.
// Appended to asset URLs so browsers cache-bust on deploy.
var AssetVer string

// Script integrity values are computed at startup from embedded JS files.
var (
	HTMXIntegrity    string
	AlpineIntegrity  string
	MermaidIntegrity string
)
