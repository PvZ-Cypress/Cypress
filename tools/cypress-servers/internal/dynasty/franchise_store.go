package dynasty

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const franchiseHeader = "FBCHUNKS"

type franchiseStore struct {
	seedFile  string
	dataDir   string
	nodePath  string
	toolPath  string
	assetRoot string
	assetSlot int
}

type franchiseMutation struct {
	temporaryPath string
	sha256        string
}

type franchiseMutationResult struct {
	SHA256 string `json:"sha256"`
}

func newFranchiseStore(cfg Config) (*franchiseStore, error) {
	if strings.TrimSpace(cfg.SeedFile) == "" {
		return nil, nil
	}
	seed, err := filepath.Abs(cfg.SeedFile)
	if err != nil {
		return nil, fmt.Errorf("resolve Dynasty seed: %w", err)
	}
	if err := validateFranchiseFile(seed); err != nil {
		return nil, fmt.Errorf("invalid Dynasty seed: %w", err)
	}
	dataDir := cfg.DataDir
	if strings.TrimSpace(dataDir) == "" {
		base := filepath.Dir(cfg.DBFile)
		if cfg.DBFile == ":memory:" || base == "." || base == "" {
			base = os.TempDir()
		}
		dataDir = filepath.Join(base, "dynasties")
	}
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve Dynasty data directory: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create Dynasty data directory: %w", err)
	}
	nodePath := defaultString(cfg.NodePath, "node")
	if resolved, lookupErr := exec.LookPath(nodePath); lookupErr == nil {
		nodePath = resolved
	} else {
		return nil, fmt.Errorf("find Node.js executable %q: %w", nodePath, lookupErr)
	}
	toolPath, err := filepath.Abs(cfg.FranchiseTool)
	if err != nil {
		return nil, fmt.Errorf("resolve franchise tool: %w", err)
	}
	if info, statErr := os.Stat(toolPath); statErr != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("franchise tool is missing: %s", toolPath)
	}
	slotRoot, err := filepath.Abs(cfg.SchemaRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve Dynasty schema root: %w", err)
	}
	slot, err := strconv.Atoi(filepath.Base(slotRoot))
	if err != nil || slot < 0 {
		return nil, fmt.Errorf("Dynasty schema root must end in a numeric asset slot: %s", slotRoot)
	}
	return &franchiseStore{
		seedFile:  seed,
		dataDir:   dataDir,
		nodePath:  nodePath,
		toolPath:  toolPath,
		assetRoot: filepath.Dir(slotRoot),
		assetSlot: slot,
	}, nil
}

func (s *franchiseStore) create(ctx context.Context, sessionID int64) (string, string, error) {
	directory := filepath.Join(s.dataDir, strconv.FormatInt(sessionID, 10))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", "", err
	}
	target := filepath.Join(directory, fmt.Sprintf("DYNASTY-%d", sessionID))
	if _, err := os.Stat(target); err == nil {
		return "", "", fmt.Errorf("Dynasty artifact already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return "", "", err
	}
	mutation, err := s.run(ctx, s.seedFile, "initialize")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(mutation.temporaryPath)
	if err := os.Rename(mutation.temporaryPath, target); err != nil {
		return "", "", err
	}
	return target, mutation.sha256, nil
}

func (s *franchiseStore) selectTeam(ctx context.Context, artifact string, teamKey, coachKey int64) (franchiseMutation, error) {
	arguments := []string{"--team-key", strconv.FormatInt(teamKey, 10)}
	if coachKey > 0 {
		arguments = append(arguments, "--coach-key", strconv.FormatInt(coachKey, 10))
	}
	return s.run(ctx, artifact, "select-team", arguments...)
}

func (s *franchiseStore) advance(ctx context.Context, artifact string, week int, stage string) (franchiseMutation, error) {
	return s.run(ctx, artifact, "advance",
		"--current-week", strconv.Itoa(week),
		"--stage", stage,
	)
}

func (s *franchiseStore) run(ctx context.Context, artifact, command string, extra ...string) (franchiseMutation, error) {
	artifact, err := filepath.Abs(artifact)
	if err != nil {
		return franchiseMutation{}, err
	}
	if err := validateFranchiseFile(artifact); err != nil {
		return franchiseMutation{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(artifact), ".next-*.DYNASTY")
	if err != nil {
		return franchiseMutation{}, err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		return franchiseMutation{}, err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return franchiseMutation{}, err
	}
	args := []string{
		s.toolPath,
		command,
		"--input", artifact,
		"--output", temporaryPath,
		"--asset-root", s.assetRoot,
		"--slot", strconv.Itoa(s.assetSlot),
	}
	args = append(args, extra...)
	process := exec.CommandContext(ctx, s.nodePath, args...)
	output, err := process.CombinedOutput()
	if err != nil {
		os.Remove(temporaryPath)
		message := strings.TrimSpace(string(output))
		if len(message) > 2048 {
			message = message[len(message)-2048:]
		}
		return franchiseMutation{}, fmt.Errorf("franchise %s failed: %w: %s", command, err, message)
	}
	if err := validateFranchiseFile(temporaryPath); err != nil {
		os.Remove(temporaryPath)
		return franchiseMutation{}, fmt.Errorf("franchise %s produced invalid output: %w", command, err)
	}
	var result franchiseMutationResult
	if err := json.Unmarshal(output, &result); err != nil {
		os.Remove(temporaryPath)
		return franchiseMutation{}, fmt.Errorf("decode franchise %s result: %w", command, err)
	}
	hash, err := sha256File(temporaryPath)
	if err != nil {
		os.Remove(temporaryPath)
		return franchiseMutation{}, err
	}
	if result.SHA256 != "" && !strings.EqualFold(result.SHA256, hash) {
		os.Remove(temporaryPath)
		return franchiseMutation{}, fmt.Errorf("franchise %s hash mismatch", command)
	}
	return franchiseMutation{temporaryPath: temporaryPath, sha256: hash}, nil
}

func swapFranchiseArtifact(current string, mutation franchiseMutation) (commit func(), rollback func(), err error) {
	backupFile, err := os.CreateTemp(filepath.Dir(current), ".previous-*.DYNASTY")
	if err != nil {
		return nil, nil, err
	}
	backupPath := backupFile.Name()
	if err := backupFile.Close(); err != nil {
		return nil, nil, err
	}
	if err := os.Remove(backupPath); err != nil {
		return nil, nil, err
	}
	if err := os.Rename(current, backupPath); err != nil {
		return nil, nil, err
	}
	if err := os.Rename(mutation.temporaryPath, current); err != nil {
		_ = os.Rename(backupPath, current)
		return nil, nil, err
	}
	committed := false
	commit = func() {
		if committed {
			return
		}
		committed = true
		_ = os.Remove(backupPath)
	}
	rollback = func() {
		if committed {
			return
		}
		committed = true
		_ = os.Remove(current)
		_ = os.Rename(backupPath, current)
	}
	return commit, rollback, nil
}

func validateFranchiseFile(file string) error {
	handle, err := os.Open(file)
	if err != nil {
		return err
	}
	defer handle.Close()
	header := make([]byte, len(franchiseHeader))
	if _, err := io.ReadFull(handle, header); err != nil {
		return err
	}
	if string(header) != franchiseHeader {
		return fmt.Errorf("%s is not an FBCHUNKS franchise save", file)
	}
	return nil
}

func sha256File(file string) (string, error) {
	handle, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer handle.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, handle); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
