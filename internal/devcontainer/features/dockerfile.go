package features

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/template"
)

// BuildContext generates a temporary build context directory containing:
//   - Dockerfile
//   - Subdirectories f0/, f1/, ... with extracted feature contents
//   - devcontainer-features-install.sh wrappers
//   - devcontainer-features.env files
//
// The caller is responsible for removing the temp directory.
// Returns the temp directory path and the path to the generated Dockerfile.
func BuildContext(baseImageRef string, features []ResolvedFeature, containerUser, remoteUser string) (contextDir string, dockerfilePath string, err error) {
	contextDir, err = os.MkdirTemp("", "dcx-features-")
	if err != nil {
		return "", "", fmt.Errorf("creating feature build context: %w", err)
	}

	for i, f := range features {
		dest := filepath.Join(contextDir, fmt.Sprintf("f%d", i))
		if err := copyDir(f.Path, dest); err != nil {
			return "", "", fmt.Errorf("copying feature %s into build context: %w", f.Meta.ID, err)
		}
		if err := writeFeatureEnvFile(filepath.Join(dest, "devcontainer-features.env"), f.Ref.Options); err != nil {
			return "", "", fmt.Errorf("writing env file for feature %s: %w", f.Meta.ID, err)
		}
		if err := writeInstallWrapper(dest); err != nil {
			return "", "", fmt.Errorf("writing install wrapper for feature %s: %w", f.Meta.ID, err)
		}
		// Ensure install.sh is executable.
		_ = os.Chmod(filepath.Join(dest, "install.sh"), 0755)
	}

	dockerfilePath = filepath.Join(contextDir, "Dockerfile")
	df, err := os.Create(dockerfilePath)
	if err != nil {
		return "", "", fmt.Errorf("creating Dockerfile: %w", err)
	}
	defer func() { _ = df.Close() }()

	if err := generateDockerfile(df, baseImageRef, features, containerUser, remoteUser); err != nil {
		return "", "", fmt.Errorf("writing Dockerfile: %w", err)
	}

	return contextDir, dockerfilePath, nil
}

type envPair struct {
	Key   string
	Value string
}

type tmplFeature struct {
	Index    int
	Meta     FeatureMeta
	Ref      FeatureRef
	EnvPairs []envPair
}

const dockerfileTmpl = `FROM {{.BaseImage}} AS dcx_features
USER root
ENV _CONTAINER_USER={{.ContainerUser}} _REMOTE_USER={{.RemoteUser}} _CONTAINER_USER_HOME={{.ContainerUserHome}} _REMOTE_USER_HOME={{.RemoteUserHome}}

{{range .TmplFeatures}}# Feature: {{.Meta.Name}} ({{.Ref.String}})
COPY ./f{{.Index}} /tmp/dcx-features/{{.Meta.ID}}/
{{range .EnvPairs}}ENV {{.Key}}={{.Value}}
{{end}}RUN cd /tmp/dcx-features/{{.Meta.ID}} && chmod +x install.sh && chmod +x devcontainer-features-install.sh && ./devcontainer-features-install.sh

{{end}}FROM dcx_features AS final
LABEL devcontainer.metadata='{{.MetadataJSON}}'
`

// generateDockerfile writes a non-BuildKit Dockerfile that installs all
// features on top of the base image.
func generateDockerfile(w io.Writer, baseImageRef string, features []ResolvedFeature, containerUser, remoteUser string) error {
	if containerUser == "" {
		containerUser = "root"
	}
	if remoteUser == "" {
		remoteUser = containerUser
	}

	meta := buildFeatureMetadata(features)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshalling feature metadata: %w", err)
	}

	tmplFeatures := make([]tmplFeature, len(features))
	for i, f := range features {
		tf := tmplFeature{
			Index: i,
			Meta:  f.Meta,
			Ref:   f.Ref,
		}
		if len(f.Meta.ContainerEnv) > 0 {
			keys := make([]string, 0, len(f.Meta.ContainerEnv))
			for k := range f.Meta.ContainerEnv {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				tf.EnvPairs = append(tf.EnvPairs, envPair{Key: k, Value: f.Meta.ContainerEnv[k]})
			}
		}
		tmplFeatures[i] = tf
	}

	data := struct {
		BaseImage         string
		ContainerUser     string
		RemoteUser        string
		ContainerUserHome string
		RemoteUserHome    string
		TmplFeatures      []tmplFeature
		MetadataJSON      string
	}{
		BaseImage:         baseImageRef,
		ContainerUser:     containerUser,
		RemoteUser:        remoteUser,
		ContainerUserHome: userHome(containerUser),
		RemoteUserHome:    userHome(remoteUser),
		TmplFeatures:      tmplFeatures,
		MetadataJSON:      string(metaJSON),
	}

	tmpl, err := template.New("dockerfile").Parse(dockerfileTmpl)
	if err != nil {
		return fmt.Errorf("parsing Dockerfile template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("executing Dockerfile template: %w", err)
	}
	if _, err := io.Copy(w, &buf); err != nil {
		return err
	}
	return nil
}

// userHome returns the typical home directory for a container user.
func userHome(user string) string {
	if user == "root" {
		return "/root"
	}
	return "/home/" + user
}

// buildFeatureMetadata creates the metadata array for the devcontainer.metadata
// label. Each feature contributes all of its properties that are relevant
// when the pre-built image is later used without repeating the feature
// configuration in devcontainer.json.
func buildFeatureMetadata(features []ResolvedFeature) []map[string]interface{} {
	result := make([]map[string]interface{}, len(features))
	for i, f := range features {
		m := map[string]interface{}{
			"id":      f.Meta.ID,
			"version": f.Meta.Version,
			"name":    f.Meta.Name,
		}
		if len(f.Ref.Options) > 0 {
			m["options"] = f.Ref.Options
		}
		if len(f.Meta.ContainerEnv) > 0 {
			m["containerEnv"] = f.Meta.ContainerEnv
		}
		if f.Meta.Init {
			m["init"] = true
		}
		if f.Meta.Privileged {
			m["privileged"] = true
		}
		if len(f.Meta.CapAdd) > 0 {
			m["capAdd"] = f.Meta.CapAdd
		}
		if len(f.Meta.SecurityOpt) > 0 {
			m["securityOpt"] = f.Meta.SecurityOpt
		}
		if f.Meta.Entrypoint != "" {
			m["entrypoint"] = f.Meta.Entrypoint
		}
		if len(f.Meta.Mounts) > 0 {
			m["mounts"] = f.Meta.Mounts
		}
		if !f.Meta.OnCreateCommand.IsEmpty() {
			m["onCreateCommand"] = f.Meta.OnCreateCommand
		}
		if !f.Meta.UpdateContentCommand.IsEmpty() {
			m["updateContentCommand"] = f.Meta.UpdateContentCommand
		}
		if !f.Meta.PostCreateCommand.IsEmpty() {
			m["postCreateCommand"] = f.Meta.PostCreateCommand
		}
		if !f.Meta.PostStartCommand.IsEmpty() {
			m["postStartCommand"] = f.Meta.PostStartCommand
		}
		if !f.Meta.PostAttachCommand.IsEmpty() {
			m["postAttachCommand"] = f.Meta.PostAttachCommand
		}
		if len(f.Meta.Customizations) > 0 {
			m["customizations"] = f.Meta.Customizations
		}
		result[i] = m
	}
	return result
}

// copyDir recursively copies the contents of src to dst using filepath.WalkDir.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
