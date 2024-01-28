package feature

// Flag is named such that checking for a feature uses `feature.Flag.Enabled(feature.ExampleFeature)`.
var Flag = New()

// flag names are written in kebab-case
const (
	ExampleFeature FlagName = "example-feature"
)

func init() {
	Flag.SetFlags(map[FlagName]FlagDesc{
		ExampleFeature: {Type: Alpha, Description: "just for testing"},
	})
}
