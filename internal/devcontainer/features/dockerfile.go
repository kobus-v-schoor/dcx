package features

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

	defer func() {
		if err != nil {
			_ = os.RemoveAll(contextDir)
		}
	}()

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
	defer df.Close()

	if err := generateDockerfile(df, baseImageRef, features, containerUser, remoteUser); err != nil {
		return "", "", fmt.Errorf("writing Dockerfile: %w", err)
	}

	return contextDir, dockerfilePath, nil
}

// generateDockerfile writes a non-BuildKit Dockerfile that installs all
// features on top of the base image.
func generateDockerfile(w io.Writer, baseImageRef string, features []ResolvedFeature, containerUser, remoteUser string) error {
	if containerUser == "" {
		containerUser = "root"
	}
	if remoteUser == "" {
		remoteUser = containerUser
	}
	containerUserHome := userHome(containerUser)
	remoteUserHome := userHome(remoteUser)

	fmt.Fprintf(w, "FROM %s AS dcx_features\n", baseImageRef)
	fmt.Fprintln(w, "USER root")
	fmt.Fprintf(w, "ENV _CONTAINER_USER=%q _REMOTE_USER=%q _CONTAINER_USER_HOME=%q _REMOTE_USER_HOME=%q\n",
		containerUser, remoteUser, containerUserHome, remoteUserHome)
	fmt.Fprintln(w)

	for i, f := range features {
		fmt.Fprintf(w, "# Feature: %s (%s)\n", f.Meta.Name, f.Ref.String())
		fmt.Fprintf(w, "COPY ./f%d /tmp/dcx-features/%s/\n", i, f.Meta.ID)
		fmt.Fprintf(w, "RUN cd /tmp/dcx-features/%s && chmod +x install.sh && chmod +x devcontainer-features-install.sh && ./devcontainer-features-install.sh\n", f.Meta.ID)
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "FROM dcx_features AS final")

	meta := buildFeatureMetadata(features)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshalling feature metadata: %w", err)
	}
	fmt.Fprintf(w, "LABEL devcontainer.metadata='%s'\n", string(metaJSON))
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

// copyDir recursively copies the contents of src to dst.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return err
			}
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile copies a single file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
