package features

import (
	"encoding/json"
	"fmt"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

// BuildJSON serializes a list of features into the JSON string expected by the
// devcontainer CLI's --additional-features flag. Feature IDs without a version
// tag have ":latest" appended. Empty or nil options are serialized as "{}"
// rather than "null". Returns an empty string when the feature list is empty.
func BuildJSON(features []config.Feature) (string, error) {
	if len(features) == 0 {
		return "", nil
	}

	m := make(map[string]map[string]interface{}, len(features))
	for _, f := range features {
		id := f.FeatureID()
		opts := f.Options
		if opts == nil {
			opts = map[string]interface{}{}
		}
		m[id] = opts
	}

	data, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("serializing additional features: %w", err)
	}

	return string(data), nil
}
