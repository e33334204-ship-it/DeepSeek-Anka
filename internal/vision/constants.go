package vision

const (
	ContextStart = "<vision-context>"
	ContextEnd   = "</vision-context>"

	VisualPrimitivesStart = "<visual-primitives"
	VisualPrimitivesEnd   = "</visual-primitives>"

	maxNoteChars            = 3200
	maxCacheEntries         = 256
	maxVisualPrimitives     = 16
	maxPrimitiveRefChars    = 96
	analysisTimeout         = 120 // seconds
	sessionNotesFile        = "session-vision-notes.json"
	defaultVisionMaxTokens  = 4096
)
