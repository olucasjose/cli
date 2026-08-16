package download

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/cli/cli/v2/internal/safeurl"
	ghzip "github.com/cli/cli/v2/internal/zip"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
)

type apiPlatform struct {
	client *http.Client
	repo   ghrepo.Interface
}

func (p *apiPlatform) List(runID string) ([]shared.Artifact, error) {
	return shared.ListArtifacts(p.client, p.repo, runID)
}

func (p *apiPlatform) Download(url safeurl.SafeURL, dir safepaths.Absolute, artifactName string) error {
	return downloadArtifact(p.client, url, dir, artifactName)
}

func downloadArtifact(httpClient *http.Client, url safeurl.SafeURL, destDir safepaths.Absolute, artifactName string) error {
	// TODO(api-client-rollout)
	// This has been deferred from moving to api.Client due to streaming the artifact ZIP response body to disk instead of decoding JSON.
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}
	// The server rejects this :(
	//req.Header.Set("Accept", "application/zip")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	if strings.HasSuffix(artifactName, ".zip") {
		tmpfile, err := os.CreateTemp("", "gh-artifact.*.zip")
		if err != nil {
			return fmt.Errorf("error initializing temporary file: %w", err)
		}
		defer func() {
			_ = tmpfile.Close()
			_ = os.Remove(tmpfile.Name())
		}()

		size, err := io.Copy(tmpfile, resp.Body)
		if err != nil {
			return fmt.Errorf("error writing zip archive: %w", err)
		}

		zipfile, err := zip.NewReader(tmpfile, size)
		if err != nil {
			return fmt.Errorf("error extracting zip archive: %w", err)
		}
		if err := ghzip.ExtractZip(zipfile, destDir); err != nil {
			return fmt.Errorf("error extracting zip archive: %w", err)
		}
	} else {
		if mkdirErr := os.MkdirAll(destDir.String(), 0755); mkdirErr != nil {
			return fmt.Errorf("error creating destination directory: %w", mkdirErr)
		}
		destPath, joinErr := destDir.Join(artifactName)
		if joinErr != nil {
			return fmt.Errorf("error building destination path: %w", joinErr)
		}
		out, createErr := os.Create(destPath.String())
		if createErr != nil {
			return fmt.Errorf("error creating destination file: %w", createErr)
		}
		defer out.Close()
		if _, copyErr := io.Copy(out, resp.Body); copyErr != nil {
			return fmt.Errorf("error writing destination file: %w", copyErr)
		}
	}

	return nil
}
