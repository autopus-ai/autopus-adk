package selfupdate

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildZip creates a zip archive from the given entries. Names ending in "/"
// become directory entries so archive layouts can be exercised directly.
func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range entries {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if name[len(name)-1] == '/' {
			header.SetMode(os.ModeDir | 0o755)
		} else {
			header.SetMode(0o755)
		}
		entry, err := writer.CreateHeader(header)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

// serveSignedRelease publishes an archive plus a matching signed checksums
// pair and returns the server together with the downloader trusting it.
func serveSignedRelease(t *testing.T, archiveName string, archive []byte) (*httptest.Server, *Downloader) {
	t.Helper()
	checksumLine := fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), archiveName)
	priv, pinned := generateReleaseTestKey(t, "2099-12-31")
	envelope := releaseSignatureEnvelope(t, []byte(checksumLine),
		testEnvelopeSigner{private: priv, fingerprint: pinned.Fingerprint})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + archiveName:
			_, _ = w.Write(archive)
		case "/checksums.txt":
			_, _ = w.Write([]byte(checksumLine))
		case "/checksums.txt.signatures":
			_, _ = w.Write(envelope)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, newDownloaderForTest([]pinnedReleaseKey{pinned}, referenceTime)
}

func downloadRelease(t *testing.T, srv *httptest.Server, dl *Downloader, archiveName, destDir string) (string, error) {
	t.Helper()
	return dl.DownloadAndVerifyWithSignature(
		srv.URL+"/"+archiveName,
		srv.URL+"/checksums.txt",
		srv.URL+"/checksums.txt.signatures",
		archiveName,
		destDir,
	)
}

// TestDownloadAndVerify_ZipArchiveExtractsExecutableBinary verifies the .zip
// branch writes the binary payload verbatim and preserves its executable bit,
// so a Windows release is not silently extracted as unusable data.
func TestDownloadAndVerify_ZipArchiveExtractsExecutableBinary(t *testing.T) {
	t.Parallel()

	archiveName := "autopus-adk_0.7.0_windows_amd64.zip"
	archive := buildZip(t, map[string]string{
		"dist/":         "",
		"dist/auto.exe": "binary-payload",
	})
	srv, dl := serveSignedRelease(t, archiveName, archive)
	destDir := t.TempDir()

	path, err := downloadRelease(t, srv, dl, archiveName, destDir)

	require.NoError(t, err)
	require.Equal(t, filepath.Join(destDir, "auto.exe"), path)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "binary-payload", string(got))
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		require.NotZero(t, info.Mode().Perm()&0o100, "extracted binary must stay executable")
	}
}

// TestDownloadAndVerify_ZipWithoutBinaryIsRejected verifies unrelated payloads
// never satisfy the extraction step; a release missing "auto" must fail loudly.
func TestDownloadAndVerify_ZipWithoutBinaryIsRejected(t *testing.T) {
	t.Parallel()

	archiveName := "autopus-adk_0.7.0_windows_amd64.zip"
	archive := buildZip(t, map[string]string{"auto/": "", "README.md": "docs"})
	srv, dl := serveSignedRelease(t, archiveName, archive)

	_, err := downloadRelease(t, srv, dl, archiveName, t.TempDir())

	require.Error(t, err)
	require.Contains(t, err.Error(), `binary "auto" not found in zip archive`)
}

// TestDownloadAndVerify_ZipEntryChecksumMismatchIsRejected verifies per-entry
// corruption inside an otherwise well-formed zip aborts extraction instead of
// leaving a silently damaged binary behind.
func TestDownloadAndVerify_ZipEntryChecksumMismatchIsRejected(t *testing.T) {
	t.Parallel()

	payload := []byte("binary-payload")
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.CreateRaw(&zip.FileHeader{
		Name:               "auto",
		Method:             zip.Store,
		CRC32:              0xdeadbeef,
		CompressedSize64:   uint64(len(payload)),
		UncompressedSize64: uint64(len(payload)),
	})
	require.NoError(t, err)
	_, err = entry.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	archiveName := "autopus-adk_0.7.0_windows_amd64.zip"
	srv, dl := serveSignedRelease(t, archiveName, buf.Bytes())

	_, err = downloadRelease(t, srv, dl, archiveName, t.TempDir())

	require.ErrorIs(t, err, zip.ErrChecksum)
}

// TestDownloadAndVerify_CorruptZipIsRejected verifies a checksum-matching but
// structurally invalid zip is refused instead of producing a partial file.
func TestDownloadAndVerify_CorruptZipIsRejected(t *testing.T) {
	t.Parallel()

	archiveName := "autopus-adk_0.7.0_windows_amd64.zip"
	srv, dl := serveSignedRelease(t, archiveName, []byte("PK\x03\x04 truncated"))
	destDir := t.TempDir()

	_, err := downloadRelease(t, srv, dl, archiveName, destDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), "open zip")
	entries, readErr := os.ReadDir(destDir)
	require.NoError(t, readErr)
	require.Empty(t, entries, "rejected archive must not leave extracted files")
}

// TestDownloadAndVerify_TruncatedTarIsRejected verifies a tar entry whose body
// is cut short fails instead of writing a truncated binary to disk.
func TestDownloadAndVerify_TruncatedTarIsRejected(t *testing.T) {
	t.Parallel()

	archiveName := "autopus-adk_0.7.0_linux_amd64.tar.gz"
	full := buildTarGz(t, "auto", strings.Repeat("payload-", 4096))
	srv, dl := serveSignedRelease(t, archiveName, full[:len(full)/2])
	destDir := t.TempDir()

	_, err := downloadRelease(t, srv, dl, archiveName, destDir)

	require.Error(t, err)
	partial, readErr := os.ReadFile(filepath.Join(destDir, "auto"))
	if readErr == nil {
		require.NotEqual(t, strings.Repeat("payload-", 4096), string(partial),
			"truncated archive must not be reported as a complete binary")
	}
}

// TestDownloadAndVerify_MissingAssetsAreReportedByStage verifies the caller can
// tell which asset was unavailable, and that a missing envelope stops the flow
// before the archive is fetched.
func TestDownloadAndVerify_MissingAssetsAreReportedByStage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		missingPath string
		wantPrefix  string
	}{
		{name: "envelope missing", missingPath: "/checksums.txt.signatures", wantPrefix: "signature envelope download"},
		{name: "archive missing", missingPath: "/autopus-adk_0.7.0_linux_amd64.tar.gz", wantPrefix: "archive download"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			archiveName := "autopus-adk_0.7.0_linux_amd64.tar.gz"
			archive := buildTarGz(t, "auto", "payload")
			checksumLine := fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), archiveName)
			priv, pinned := generateReleaseTestKey(t, "2099-12-31")
			envelope := releaseSignatureEnvelope(t, []byte(checksumLine),
				testEnvelopeSigner{private: priv, fingerprint: pinned.Fingerprint})

			var archiveRequests int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == test.missingPath {
					http.Error(w, "gone", http.StatusNotFound)
					return
				}
				switch r.URL.Path {
				case "/" + archiveName:
					archiveRequests++
					_, _ = w.Write(archive)
				case "/checksums.txt":
					_, _ = w.Write([]byte(checksumLine))
				case "/checksums.txt.signatures":
					_, _ = w.Write(envelope)
				}
			}))
			defer srv.Close()

			dl := newDownloaderForTest([]pinnedReleaseKey{pinned}, referenceTime)
			_, err := downloadRelease(t, srv, dl, archiveName, t.TempDir())

			require.Error(t, err)
			require.Contains(t, err.Error(), test.wantPrefix)
			require.Contains(t, err.Error(), "HTTP 404")
			if test.missingPath == "/checksums.txt.signatures" {
				require.Zero(t, archiveRequests, "archive must not be fetched before the envelope verifies")
			}
		})
	}
}

// TestDownloadAndVerify_UnusableChecksumURLFailsBeforeNetwork verifies the
// four-argument API refuses a checksums URL it cannot derive an envelope from.
func TestDownloadAndVerify_UnusableChecksumURLFailsBeforeNetwork(t *testing.T) {
	t.Parallel()

	dl := NewDownloader()
	_, err := dl.DownloadAndVerify(
		"https://example.test/archive.tar.gz",
		"https://example.test/release/SHA256SUMS",
		"archive.tar.gz",
		t.TempDir(),
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot derive release signatures URL")
}

// TestHTTPGetWithRetry_TransportFailuresExhaustAttempts verifies unreachable
// and unbuildable URLs surface the last transport error after the retry budget
// rather than returning empty data.
func TestHTTPGetWithRetry_TransportFailuresExhaustAttempts(t *testing.T) {
	t.Parallel()

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	tests := []struct {
		name string
		url  string
	}{
		{name: "malformed url", url: "http://exa mple.test/checksums.txt"},
		{name: "connection refused", url: closedURL + "/checksums.txt"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			data, err := httpGetWithRetry(test.url, maxChecksumSize)

			require.Error(t, err)
			require.Nil(t, data)
			require.Contains(t, err.Error(), fmt.Sprintf("failed after %d attempts", downloadRetries))
		})
	}
}
