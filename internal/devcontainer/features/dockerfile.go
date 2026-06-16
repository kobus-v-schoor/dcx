package features

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

const dockerfileTmpl = `FROM {{.BaseImage}} AS dcx_features
USER root
ENV _CONTAINER_USER={{.ContainerUser}} _REMOTE_USER={{.RemoteUser}} _CONTAINER_USER_HOME={{.ContainerUserHome}} _REMOTE_USER_HOME={{.RemoteUserHome}}

{{range $i, $f := .Features}}# Feature: {{$f.Meta.Name}} ({{$f.Ref.String}})
COPY ./f{{$i}} /tmp/dcx-features/{{$f.Meta.ID}}/
RUN cd /tmp/dcx-features/{{$f.Meta.ID}} && chmod +x install.sh && chmod +x devcontainer-features-install.sh && ./devcontainer-features-install.sh

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

	data := struct {
		BaseImage         string
		ContainerUser     string
		RemoteUser        string
		ContainerUserHome string
		RemoteUserHome    string
		Features          []ResolvedFeature
		MetadataJSON      string
	}{
		BaseImage:         baseImageRef,
		ContainerUser:     containerUser,
		RemoteUser:        remoteUser,
		ContainerUserHome: userHome(containerUser),
		RemoteUserHome:    userHome(remoteUser),
		Features:          features,
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
// label. Each feature contributes its id, version, name, containerEnv,
// and merged options.
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
