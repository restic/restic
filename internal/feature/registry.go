package feature

// Flag is named such that checking for a feature uses `feature.Flag.Enabled(feature.ExampleFeature)`.
var Flag = New()

// flag names are written in kebab-case
const (
	ExampleFeature       FlagName = "example-feature"
	DeprecateLegacyIndex FlagName = "deprecate-legacy-index"
)

func init() {
	Flag.SetFlags(map[FlagName]FlagDesc{
		ExampleFeature:       {Type: Alpha, Description: "just for testing"},
		DeprecateLegacyIndex: {Type: Beta, Description: "disable support for index format used by restic 0.1.0. Use `restic repair index` to update the index if necessary."},
	})
}
