package cli

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/git"

	"golang.org/x/crypto/ssh"
)

const (
	sandboxDirectiveLockRequested = "__rwx_sandbox_lock_requested__"
	sandboxDirectiveLockReleased  = "__rwx_sandbox_lock_released__"
)

// Config types

type StartSandboxConfig struct {
	ConfigFile     string
	RunID          string
	RwxDirectory   string
	Json           bool
	Wait           bool
	InitParameters map[string]string
	// storageLock is an already-held storage lock passed by the caller
	// (e.g. ExecSandbox) to keep the "check-then-create" atomic.
	// StartSandbox will release it after persisting the initial session.
	storageLock *SandboxStorageLock
}

// StartSandboxConfigWithLock returns a copy of cfg with the storage lock set.
// This is exported for testing; production callers set storageLock directly.
func StartSandboxConfigWithLock(cfg StartSandboxConfig, lock *SandboxStorageLock) StartSandboxConfig {
	cfg.storageLock = lock
	return cfg
}

type ExecSandboxConfig struct {
	ConfigFile     string
	Command        []string
	RunID          string
	RwxDirectory   string
	Json           bool
	Sync           bool
	InitParameters map[string]string
	Reset          bool
}

type ListSandboxesConfig struct {
	Json bool
}

type StopSandboxConfig struct {
	RunID string
	All   bool
	Json  bool
}

type ResetSandboxConfig struct {
	ConfigFile     string
	RwxDirectory   string
	Json           bool
	Wait           bool
	InitParameters map[string]string
}

// Result types

type StartSandboxResult struct {
	RunID      string
	RunURL     string
	ConfigFile string
}

type ExecSandboxResult struct {
	RunID       string
	ExitCode    int
	RunURL      string
	PulledFiles []string
}

type ListSandboxesResult struct {
	Sandboxes []SandboxInfo
}

type SandboxInfo struct {
	RunID      string
	Status     string
	ConfigFile string
	Branch     string
}

type StopSandboxResult struct {
	Stopped []StoppedSandbox
}

type StoppedSandbox struct {
	RunID      string
	WasRunning bool
}

type ResetSandboxResult struct {
	OldRunID string
	NewRunID string
	RunURL   string
}

type GetSandboxInitTemplateConfig struct {
	Json bool
}

type GetSandboxInitTemplateResult struct {
	Template string
}

// Service methods

func (s Service) GetSandboxInitTemplate(cfg GetSandboxInitTemplateConfig) (*GetSandboxInitTemplateResult, error) {
	result, err := s.APIClient.GetSandboxInitTemplate()
	if err != nil {
		return nil, errors.Wrap(err, "unable to fetch sandbox init template")
	}

	return &GetSandboxInitTemplateResult{
		Template: result.Template,
	}, nil
}

type CheckExistingSandboxResult struct {
	Exists     bool
	Active     bool
	RunID      string
	RunURL     string
	ConfigFile string
}

func (s Service) CheckExistingSandbox(configFile string) (*CheckExistingSandboxResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get current directory")
	}
	branch := GetCurrentGitBranch(cwd)

	storage, err := LoadSandboxStorage()
	if err != nil {
		return &CheckExistingSandboxResult{Exists: false}, nil
	}

	session, found := storage.GetSession(branch, configFile)
	if !found && IsDetachedBranch(branch) {
		gitClient := &git.Client{Binary: "git", Dir: cwd}
		session, found = storage.GetSessionByAncestry(branch, configFile, gitClient)
		if found {
			_ = storage.Save()
		}
	}
	if !found {
		return &CheckExistingSandboxResult{Exists: false}, nil
	}

	// Check if the run is still active (use scoped token if available)
	connInfo, err := s.APIClient.GetSandboxConnectionInfo(session.RunID, session.ScopedToken)
	if err != nil {
		// Can't check status, treat as not active
		return &CheckExistingSandboxResult{
			Exists:     true,
			Active:     false,
			RunID:      session.RunID,
			ConfigFile: session.ConfigFile,
		}, nil
	}

	if connInfo.Polling.Completed && !connInfo.Sandboxable {
		// Run finished without becoming sandboxable — not active
		return &CheckExistingSandboxResult{
			Exists:     true,
			Active:     false,
			RunID:      session.RunID,
			ConfigFile: session.ConfigFile,
		}, nil
	}

	runURL := s.sandboxRunURL(session)
	return &CheckExistingSandboxResult{
		Exists:     true,
		Active:     true,
		RunID:      session.RunID,
		RunURL:     runURL,
		ConfigFile: session.ConfigFile,
	}, nil
}

func (s Service) StartSandbox(cfg StartSandboxConfig) (*StartSandboxResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get current directory")
	}
	branch := GetCurrentGitBranch(cwd)

	// If --id is provided, check if run is still active and reattach
	if cfg.RunID != "" {
		// Release caller-provided lock since the --id path manages its own locking
		if cfg.storageLock != nil {
			UnlockSandboxStorage(cfg.storageLock)
		}
		// Check if we have an existing session with a scoped token
		var existingScopedToken string
		storage, err := LoadSandboxStorage()
		if err == nil {
			if existingSession, _, found := storage.FindByRunID(cfg.RunID); found {
				existingScopedToken = existingSession.ScopedToken
			}
		}

		connInfo, err := s.APIClient.GetSandboxConnectionInfo(cfg.RunID, existingScopedToken)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get sandbox info for %s", cfg.RunID)
		}

		if connInfo.Polling.Completed && !connInfo.Sandboxable {
			return nil, fmt.Errorf("Run '%s' is no longer active (timed out or cancelled).\nRun 'rwx sandbox start %s' to create a new sandbox.", cfg.RunID, cfg.ConfigFile)
		}

		// Only wait for sandbox to be ready if --wait flag is set
		if cfg.Wait && !connInfo.Sandboxable {
			if _, err := s.waitForSandboxReadyWithToken(cfg.RunID, existingScopedToken, cfg.Json); err != nil {
				return nil, err
			}
		}

		// Store session if not already stored, creating a scoped token if needed
		if storage != nil {
			if _, _, found := storage.FindByRunID(cfg.RunID); !found {
				// Create a scoped token for this reattached session
				var scopedToken string
				tokenResult, err := s.APIClient.CreateSandboxToken(api.CreateSandboxTokenConfig{
					RunID: cfg.RunID,
				})
				if err != nil {
					fmt.Fprintf(s.Stderr, "Warning: Unable to create scoped token: %v\n", err)
				} else {
					scopedToken = tokenResult.Token
				}

				lockFile, lockErr := s.lockSandboxStorageWithInfo(cfg.Json)
				if lockErr != nil {
					fmt.Fprintf(s.Stderr, "Warning: Unable to lock sandbox storage: %v\n", lockErr)
				} else {
					// Reload under lock to avoid overwriting concurrent writes
					storage, err = LoadSandboxStorage()
					if err != nil {
						fmt.Fprintf(s.Stderr, "Warning: Unable to load sandbox sessions: %v\n", err)
					} else {
						storage.SetSession(branch, cfg.ConfigFile, SandboxSession{
							RunID:       cfg.RunID,
							ConfigFile:  cfg.ConfigFile,
							ScopedToken: scopedToken,
							ConfigHash:  HashConfigFile(cfg.ConfigFile),
						})
						if err := storage.Save(); err != nil {
							fmt.Fprintf(s.Stderr, "Warning: Unable to save sandbox session: %v\n", err)
						}
					}
					UnlockSandboxStorage(lockFile)
				}
			}
		}

		runURL := s.sandboxRunURL(nil)
		if !cfg.Json {
			if runURL != "" {
				fmt.Fprintf(s.Stdout, "Attached to sandbox: %s\n%s\n", cfg.RunID, runURL)
			} else {
				fmt.Fprintf(s.Stdout, "Attached to sandbox: %s\n", cfg.RunID)
			}
		}

		s.recordTelemetry("sandbox.start", map[string]any{
			"reuse": true,
		})

		return &StartSandboxResult{
			RunID:      cfg.RunID,
			RunURL:     runURL,
			ConfigFile: cfg.ConfigFile,
		}, nil
	}

	// Start a new sandbox run
	var finishSpinner func(string)
	if !cfg.Json {
		finishSpinner = SpinUntilDone("Starting sandbox...", s.StdoutIsTTY, s.Stdout)
	}

	// Construct a descriptive title for the sandbox run
	title := SandboxTitle(cwd, branch, cfg.ConfigFile)

	runResult, err := s.InitiateRun(InitiateRunConfig{
		MintFilePath:   cfg.ConfigFile,
		RwxDirectory:   cfg.RwxDirectory,
		Json:           cfg.Json,
		Title:          title,
		InitParameters: cfg.InitParameters,
		Patchable:      true,
		CliState:       EncodeCliState(branch, cfg.ConfigFile),
	})

	if err != nil {
		if finishSpinner != nil {
			finishSpinner("Failed to start sandbox")
		}
		if cfg.storageLock != nil {
			UnlockSandboxStorage(cfg.storageLock)
		}
		return nil, err
	}

	if finishSpinner != nil {
		finishSpinner(fmt.Sprintf("Started sandbox: %s\n%s", runResult.RunID, runResult.RunURL))
	}

	// Persist session immediately so sandbox list can find this run even if the
	// process crashes before we finish setup (e.g. during scoped token creation).
	// Use the caller-provided lock if available to keep check-then-create atomic.
	lockFile := cfg.storageLock
	if lockFile == nil {
		var lockErr error
		lockFile, lockErr = s.lockSandboxStorageWithInfo(cfg.Json)
		if lockErr != nil {
			fmt.Fprintf(s.Stderr, "Warning: Unable to lock sandbox storage: %v\n", lockErr)
		}
	}

	now := time.Now().UTC()
	storage, err := LoadSandboxStorage()
	if err != nil {
		fmt.Fprintf(s.Stderr, "Warning: Unable to load sandbox sessions: %v\n", err)
	} else {
		storage.SetSession(branch, cfg.ConfigFile, SandboxSession{
			RunID:      runResult.RunID,
			ConfigFile: cfg.ConfigFile,
			RunURL:     runResult.RunURL,
			ConfigHash: HashConfigFile(cfg.ConfigFile),
			CreatedAt:  &now,
		})
		if err := storage.Save(); err != nil {
			fmt.Fprintf(s.Stderr, "Warning: Unable to save sandbox session: %v\n", err)
		}
	}

	UnlockSandboxStorage(lockFile)

	// Request a scoped token for this run
	var scopedToken string
	tokenResult, err := s.APIClient.CreateSandboxToken(api.CreateSandboxTokenConfig{
		RunID: runResult.RunID,
	})
	if err != nil {
		fmt.Fprintf(s.Stderr, "Warning: Unable to create scoped token: %v\n", err)
	} else {
		scopedToken = tokenResult.Token
	}

	// Update session with scoped token now that we have it
	if storage != nil {
		var lockErr error
		lockFile, lockErr = s.lockSandboxStorageWithInfo(cfg.Json)
		if lockErr != nil {
			fmt.Fprintf(s.Stderr, "Warning: Unable to lock sandbox storage: %v\n", lockErr)
		}

		// Reload under lock to avoid overwriting concurrent writes
		storage, err = LoadSandboxStorage()
		if err != nil {
			fmt.Fprintf(s.Stderr, "Warning: Unable to load sandbox sessions: %v\n", err)
		} else {
			storage.SetSession(branch, cfg.ConfigFile, SandboxSession{
				RunID:       runResult.RunID,
				ConfigFile:  cfg.ConfigFile,
				ScopedToken: scopedToken,
				RunURL:      runResult.RunURL,
				ConfigHash:  HashConfigFile(cfg.ConfigFile),
				CreatedAt:   &now,
			})
			if err := storage.Save(); err != nil {
				fmt.Fprintf(s.Stderr, "Warning: Unable to save sandbox session: %v\n", err)
			}
		}

		UnlockSandboxStorage(lockFile)
	}

	s.recordTelemetry("sandbox.start", map[string]any{
		"reuse": false,
	})

	// Build result now so we can return it even if waiting fails
	result := &StartSandboxResult{
		RunID:      runResult.RunID,
		RunURL:     runResult.RunURL,
		ConfigFile: cfg.ConfigFile,
	}

	// Only wait for sandbox to be ready if --wait flag is set
	if cfg.Wait {
		_, err = s.waitForSandboxReadyWithToken(runResult.RunID, scopedToken, cfg.Json)
		if err != nil {
			// Return result WITH error so caller can still use the URL
			return result, err
		}
	}

	return result, nil
}

func (s Service) ExecSandbox(cfg ExecSandboxConfig) (*ExecSandboxResult, error) {
	execStart := time.Now()
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get current directory")
	}
	branch := GetCurrentGitBranch(cwd)

	var runID string
	var configFile string
	var scopedToken string
	var sessionRunURL string
	var storedConfigHash string
	var execCount int
	var resetNagShown bool
	isNewSandbox := false

	// Sandbox selection priority:
	// 1. --id flag
	// 2. Find by CWD + git branch in storage
	// 3. Auto-create new sandbox

	if cfg.RunID != "" {
		// Use specified run ID directly - waitForSandboxReadyWithToken will check if it's valid
		runID = cfg.RunID
		configFile = cfg.ConfigFile

		// Look up scoped token from storage if session exists
		storage, err := LoadSandboxStorage()
		if err == nil {
			if existingSession, _, found := storage.FindByRunID(cfg.RunID); found {
				scopedToken = existingSession.ScopedToken
				sessionRunURL = existingSession.RunURL
				storedConfigHash = existingSession.ConfigHash
			}
		}
	} else {
		// Serialize sandbox resolution across concurrent CLI processes.
		// The lock is released as soon as a run ID is determined so that
		// the actual SSH exec can proceed concurrently (serialized by the
		// agent-side lock instead).
		lockFile, lockErr := s.lockSandboxStorageWithInfo(cfg.Json)
		if lockErr != nil {
			return nil, errors.Wrap(lockErr, "unable to lock sandbox storage")
		}

		// Try to find existing session
		storage, err := LoadSandboxStorage()
		if err != nil {
			fmt.Fprintf(s.Stderr, "Warning: Unable to load sandbox sessions: %v\n", err)
			storage = &SandboxStorage{Sandboxes: make(map[string]SandboxSession)}
		}

		var session *SandboxSession
		found := false

		if cfg.ConfigFile != "" {
			// Config file provided - look up specific session
			session, found = storage.GetSession(branch, cfg.ConfigFile)
			if !found && IsDetachedBranch(branch) {
				gitClient := &git.Client{Binary: "git", Dir: cwd}
				session, found = storage.GetSessionByAncestry(branch, cfg.ConfigFile, gitClient)
				if found {
					_ = storage.Save()
				}
			}
			if found {
				// Check if session is still valid (use scoped token if available).
				// Polling.Completed=true with Sandboxable=true is a ready sandbox,
				// not an expired one; only prune when the run finished without becoming sandboxable.
				connInfo, err := s.APIClient.GetSandboxConnectionInfo(session.RunID, session.ScopedToken)
				if err != nil {
					storage.DeleteSession(branch, cfg.ConfigFile)
					_ = storage.Save()
					found = false
				} else if connInfo.Polling.Completed && !connInfo.Sandboxable {
					storage.DeleteSession(branch, cfg.ConfigFile)
					_ = storage.Save()
					found = false
				} else {
					runID = session.RunID
					configFile = session.ConfigFile
					scopedToken = session.ScopedToken
					sessionRunURL = session.RunURL
					storedConfigHash = session.ConfigHash
					execCount = session.ExecCount
					resetNagShown = session.ResetNagShown
				}
			}
		} else {
			// No config file - find any session for this branch
			sessions := storage.GetSessionsForBranch(branch)
			if len(sessions) == 0 && IsDetachedBranch(branch) {
				gitClient := &git.Client{Binary: "git", Dir: cwd}
				sessions = storage.GetSessionsForBranchByAncestry(branch, gitClient)
				if len(sessions) > 0 {
					_ = storage.Save()
				}
			}

			// Filter to only active sessions. A ready sandbox reports
			// Polling.Completed=true with Sandboxable=true; only prune when
			// the run finished without becoming sandboxable.
			var activeSessions []SandboxSession
			for _, sess := range sessions {
				connInfo, err := s.APIClient.GetSandboxConnectionInfo(sess.RunID, sess.ScopedToken)
				if err == nil && (connInfo.Sandboxable || !connInfo.Polling.Completed) {
					activeSessions = append(activeSessions, sess)
				} else {
					// Clean up expired session
					storage.DeleteSession(branch, sess.ConfigFile)
				}
			}
			_ = storage.Save()

			if len(activeSessions) == 1 {
				runID = activeSessions[0].RunID
				configFile = activeSessions[0].ConfigFile
				scopedToken = activeSessions[0].ScopedToken
				sessionRunURL = activeSessions[0].RunURL
				storedConfigHash = activeSessions[0].ConfigHash
				execCount = activeSessions[0].ExecCount
				resetNagShown = activeSessions[0].ResetNagShown
				found = true
			} else if len(activeSessions) > 1 {
				UnlockSandboxStorage(lockFile)
				return nil, fmt.Errorf("Multiple active sandboxes found for branch %s.\nSpecify a config file to select one, or use --id to specify a run ID.", branch)
			}
		}

		// Resolve config file once for both remote recovery and auto-create
		cfgFile := cfg.ConfigFile
		if cfgFile == "" {
			cfgFile = FindDefaultSandboxConfigFile()
		}

		if !found {
			// Check if a matching sandbox already exists remotely
			listResult, listErr := s.APIClient.ListSandboxRuns()
			if listErr == nil {
				for _, run := range listResult.Runs {
					if run.CliState == "" {
						continue
					}
					state, decErr := DecodeCliState(run.CliState)
					if decErr != nil {
						continue
					}
					branchMatch := state.Branch == branch
					if !branchMatch && IsDetachedBranch(branch) && IsDetachedBranch(state.Branch) {
						storedSHA := DetachedShortSHA(state.Branch)
						if storedSHA != "" {
							gitClient := &git.Client{Binary: "git", Dir: cwd}
							branchMatch = gitClient.IsAncestor(storedSHA, "HEAD")
						}
					}
					if branchMatch && state.ConfigFile == cfgFile {
						// Verify the remote sandbox is still alive before reusing.
						// A ready sandbox reports Polling.Completed=true with Sandboxable=true;
						// only skip when the run finished without becoming sandboxable.
						connInfo, connErr := s.APIClient.GetSandboxConnectionInfo(run.ID, "")
						if connErr != nil || (connInfo.Polling.Completed && !connInfo.Sandboxable) {
							continue
						}

						runID = run.ID
						configFile = cfgFile
						sessionRunURL = run.RunURL

						// Create a scoped token for this recovered session
						tokenResult, tokenErr := s.APIClient.CreateSandboxToken(api.CreateSandboxTokenConfig{
							RunID: run.ID,
						})
						if tokenErr != nil {
							fmt.Fprintf(s.Stderr, "Warning: Unable to create scoped token: %v\n", tokenErr)
						} else {
							scopedToken = tokenResult.Token
						}

						// Store locally so future execs find it without an API call
						storage.SetSession(branch, cfgFile, SandboxSession{
							RunID:       run.ID,
							ConfigFile:  cfgFile,
							ScopedToken: scopedToken,
							RunURL:      run.RunURL,
							ConfigHash:  HashConfigFile(cfgFile),
						})
						if saveErr := storage.Save(); saveErr != nil {
							fmt.Fprintf(s.Stderr, "Warning: Unable to save sandbox session: %v\n", saveErr)
						}

						found = true
						break
					}
				}
			}
		}

		if found && cfg.Reset {
			connInfo, connErr := s.APIClient.GetSandboxConnectionInfo(runID, scopedToken)
			if connErr == nil && connInfo.Sandboxable {
				if sshErr := s.connectSSH(&connInfo); sshErr == nil {
					_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_end__")
					s.SSHClient.Close()
				} else {
					if cancelErr := s.APIClient.CancelRun(runID, scopedToken); cancelErr != nil {
						fmt.Fprintf(s.Stderr, "Warning: failed to cancel run %s: %v\n", runID, cancelErr)
					}
				}
			} else if connErr == nil && !connInfo.Polling.Completed {
				if cancelErr := s.APIClient.CancelRun(runID, scopedToken); cancelErr != nil {
					fmt.Fprintf(s.Stderr, "Warning: failed to cancel run %s: %v\n", runID, cancelErr)
				}
			}
			s.waitForSandboxCompletion(runID, scopedToken)
			storage.DeleteSession(branch, configFile)
			if saveErr := storage.Save(); saveErr != nil {
				fmt.Fprintf(s.Stderr, "Warning: unable to save sandbox sessions: %v\n", saveErr)
			}
			found = false
			runID = ""
			configFile = ""
			scopedToken = ""
			sessionRunURL = ""
			storedConfigHash = ""
		}

		if !found {
			// Pass the lock to StartSandbox so the "no session found → create
			// new sandbox → persist session" sequence is atomic. StartSandbox
			// will release it after the initial session is saved.
			isNewSandbox = true
			startResult, err := s.StartSandbox(StartSandboxConfig{
				ConfigFile:     cfgFile,
				RwxDirectory:   cfg.RwxDirectory,
				Json:           cfg.Json,
				InitParameters: cfg.InitParameters,
				storageLock:    lockFile,
			})
			if err != nil {
				return nil, err
			}
			runID = startResult.RunID
			configFile = startResult.ConfigFile

			// Load the newly created session to get the scoped token
			storage, err = LoadSandboxStorage()
			if err == nil {
				if newSession, ok := storage.GetSession(branch, cfgFile); ok {
					scopedToken = newSession.ScopedToken
				}
			}
		} else {
			// Resolution complete — release the file lock so concurrent execs
			// can proceed. The agent-side lock serializes from here.
			UnlockSandboxStorage(lockFile)
		}
	}

	if !isNewSandbox && execCount >= 1 && !resetNagShown {
		fmt.Fprintf(s.Stderr, "Reconnecting to existing sandbox. To re-run setup tasks, use: rwx sandbox exec --reset -- <command>\n")
		if nagLock, nagLockErr := s.lockSandboxStorageWithInfo(cfg.Json); nagLockErr == nil {
			if nagStorage, nagLoadErr := LoadSandboxStorage(); nagLoadErr == nil {
				if nagSession, ok := nagStorage.GetSession(branch, configFile); ok {
					nagSession.ResetNagShown = true
					nagStorage.SetSession(branch, configFile, *nagSession)
					_ = nagStorage.Save()
				}
			}
			UnlockSandboxStorage(nagLock)
		}
	}

	// Warn if the sandbox definition has changed since this sandbox was started
	if storedConfigHash != "" && configFile != "" {
		currentHash := HashConfigFile(configFile)
		if currentHash != "" && currentHash != storedConfigHash {
			fmt.Fprintf(s.Stderr, "Warning: %s has changed since this sandbox was started.\nThe running sandbox does not reflect these changes.\nRun 'rwx sandbox reset' to apply the new definition.\n\n", configFile)
		}
	}

	// Get connection info (use scoped token if available)
	connInfo, err := s.waitForSandboxReadyWithToken(runID, scopedToken, cfg.Json)
	if err != nil {
		return nil, err
	}

	// Connect via SSH
	err = s.connectSSH(connInfo)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			port := "43215"
			if _, p, splitErr := net.SplitHostPort(connInfo.Address); splitErr == nil {
				port = p
			}
			return nil, fmt.Errorf("Failed to connect to sandbox '%s': %v\nThis is likely caused by a firewall or network configuration blocking outbound connections on port %s.", runID, err, port)
		}
		return nil, fmt.Errorf("Failed to connect to sandbox '%s': %v\nThe sandbox may have timed out. Run 'rwx sandbox reset %s' to restart.", runID, err, configFile)
	}
	defer s.SSHClient.Close()

	// Acquire the distributed lock so concurrent exec calls on the same
	// sandbox are serialized by the agent. Blocks until the lock is granted.
	// Show a spinner if another exec is holding the lock.
	lockDone := make(chan struct{})
	if !cfg.Json {
		go func() {
			t := time.NewTimer(500 * time.Millisecond)
			defer t.Stop()
			select {
			case <-lockDone:
				return
			case <-t.C:
				stopSpinner := Spin("Waiting for another sandbox exec to complete...", s.StderrIsTTY, s.Stderr)
				<-lockDone
				stopSpinner()
			}
		}()
	}
	_, lockErr := s.SSHClient.ExecuteCommand(sandboxDirectiveLockRequested)
	close(lockDone)
	if lockErr != nil {
		return nil, errors.Wrap(lockErr, "failed to acquire sandbox lock")
	}
	defer func() {
		_, _ = s.SSHClient.ExecuteCommand(sandboxDirectiveLockReleased)
	}()

	// Clean up any dirty state from a previous interrupted exec.
	// This makes exec self-healing — no manual reset needed after crashes.
	// Skip on new sandboxes: no prior exec could have left dirty state, and
	// checkout+clean would wipe setup artifacts written by the setup tasks.
	if cfg.Sync && !isNewSandbox {
		if cleanErr := s.cleanSandboxState(); cleanErr != nil {
			fmt.Fprintf(s.Stderr, "Warning: failed to clean sandbox state: %v\n", cleanErr)
		}
	}

	// Sync local changes to sandbox if enabled
	var syncPushMs int64
	var syncPushPatchBytes int
	if cfg.Sync {
		syncPushStart := time.Now()
		patchBytes, err := s.syncChangesToSandbox(cfg.Json, isNewSandbox)
		syncPushMs = time.Since(syncPushStart).Milliseconds()
		syncPushPatchBytes = patchBytes
		if err != nil {
			if errors.Is(err, errors.ErrSandboxNoGitDir) {
				// Stop the sandbox so the user gets a fresh one on retry
				if _, endErr := s.SSHClient.ExecuteCommand("__rwx_sandbox_end__"); endErr != nil {
					fmt.Fprintf(s.Stderr, "Warning: failed to stop sandbox: %v\n", endErr)
				}
				if lockFile, lockErr := s.lockSandboxStorageWithInfo(cfg.Json); lockErr == nil {
					if storage, loadErr := LoadSandboxStorage(); loadErr == nil {
						storage.DeleteSessionByRunID(runID)
						if saveErr := storage.Save(); saveErr != nil {
							fmt.Fprintf(s.Stderr, "Warning: failed to remove sandbox session: %v\n", saveErr)
						}
					} else {
						fmt.Fprintf(s.Stderr, "Warning: failed to remove sandbox session: %v\n", loadErr)
					}
					UnlockSandboxStorage(lockFile)
				} else {
					fmt.Fprintf(s.Stderr, "Warning: failed to lock sandbox storage: %v\n", lockErr)
				}
			}
			return nil, errors.Wrap(err, "failed to sync changes to sandbox")
		}
	}

	// Execute command — shell-quote each argument so the remote shell
	// preserves the original grouping (e.g. bash -c "cat README.md").
	command := shellescape.QuoteCommand(cfg.Command)
	cmdStart := time.Now()
	exitCode, err := s.SSHClient.ExecuteCommand(command)
	cmdDuration := time.Since(cmdStart).Milliseconds()
	s.recordTelemetry("ssh.command", map[string]any{
		"duration_ms": cmdDuration,
		"exit_code":   exitCode,
		"interactive": false,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute command in sandbox")
	}

	// Pull changes from sandbox back to local
	var pulledFiles []string
	var syncPullMs int64
	var syncPullPatchBytes int
	if cfg.Sync {
		var syncPullSuccess bool
		var syncPullRejCount int
		pullStart := time.Now()
		pulled, pullPatchBytes, pullErr := s.pullChangesFromSandbox(cwd, cfg.Json)
		syncPullMs = time.Since(pullStart).Milliseconds()
		syncPullPatchBytes = pullPatchBytes
		if pullErr != nil {
			fmt.Fprintf(s.Stderr, "Warning: failed to pull changes from sandbox: %v\n", pullErr)
			syncPullRejCount = len(findRejFiles(cwd, pulled))
		} else {
			syncPullSuccess = true
		}
		if pulled != nil {
			pulledFiles = pulled
		}

		s.recordTelemetry("sandbox.sync_pull", map[string]any{
			"patch_bytes":    pullPatchBytes,
			"duration_ms":    syncPullMs,
			"success":        syncPullSuccess,
			"rej_file_count": syncPullRejCount,
		})
	}

	// Revert sandbox to clean HEAD so the next exec starts from a known state
	if revertErr := s.revertSandbox(); revertErr != nil {
		fmt.Fprintf(s.Stderr, "Warning: failed to revert sandbox: %v\n", revertErr)
	}

	// Update session exec count and last exec time
	execNow := time.Now().UTC()
	if lockFile, lockErr := s.lockSandboxStorageWithInfo(cfg.Json); lockErr == nil {
		if storage, loadErr := LoadSandboxStorage(); loadErr == nil {
			if session, ok := storage.GetSession(branch, configFile); ok {
				session.LastExecAt = &execNow
				session.ExecCount++
				storage.SetSession(branch, configFile, *session)
				_ = storage.Save()
			}
		}
		UnlockSandboxStorage(lockFile)
	}

	s.recordTelemetry("sandbox.exec", map[string]any{
		"duration_ms":      time.Since(execStart).Milliseconds(),
		"exit_code":        exitCode,
		"sync_push_ms":     syncPushMs,
		"sync_pull_ms":     syncPullMs,
		"push_patch_bytes": syncPushPatchBytes,
		"pull_patch_bytes": syncPullPatchBytes,
	})

	runURL := s.sandboxRunURL(&SandboxSession{RunURL: sessionRunURL})
	return &ExecSandboxResult{RunID: runID, ExitCode: exitCode, RunURL: runURL, PulledFiles: pulledFiles}, nil
}

func (s Service) ListSandboxes(cfg ListSandboxesConfig) (*ListSandboxesResult, error) {
	lockFile, lockErr := s.lockSandboxStorageWithInfo(cfg.Json)
	if lockErr != nil {
		return nil, errors.Wrap(lockErr, "unable to lock sandbox storage")
	}
	defer UnlockSandboxStorage(lockFile)

	storage, err := LoadSandboxStorage()
	if err != nil {
		return nil, errors.Wrap(err, "unable to load sandbox sessions")
	}

	listResult, err := s.APIClient.ListSandboxRuns()
	if err != nil {
		return nil, errors.Wrap(err, "unable to list sandbox runs")
	}

	// Build set of active run IDs from API response
	activeRuns := make(map[string]api.SandboxRunSummary, len(listResult.Runs))
	for _, run := range listResult.Runs {
		activeRuns[run.ID] = run
	}

	// Merge remotely-discovered runs into local storage
	storageChanged := false
	for _, run := range listResult.Runs {
		if run.CliState == "" {
			continue
		}
		state, err := DecodeCliState(run.CliState)
		if err != nil {
			continue
		}
		// Only create a local session if none exists for this key
		if _, exists := storage.GetSession(state.Branch, state.ConfigFile); exists {
			continue
		}
		storage.SetSession(state.Branch, state.ConfigFile, SandboxSession{
			RunID:      run.ID,
			ConfigFile: state.ConfigFile,
			RunURL:     run.RunURL,
		})
		storageChanged = true
	}

	// Walk local sessions: keep active, prune expired
	sandboxes := make([]SandboxInfo, 0, len(storage.Sandboxes))
	var expiredKeys []string

	for key, session := range storage.AllSessions() {
		branch, _ := ParseSessionKey(key)

		if _, active := activeRuns[session.RunID]; active {
			sandboxes = append(sandboxes, SandboxInfo{
				RunID:      session.RunID,
				Status:     "active",
				ConfigFile: session.ConfigFile,
				Branch:     branch,
			})
		} else {
			// Not in the bulk list — could be initializing or genuinely expired.
			// Verify individually: only keep as active if the API positively
			// confirms the run is still alive. A ready sandbox reports
			// Polling.Completed=true with Sandboxable=true.
			status := "expired"
			connInfo, connErr := s.APIClient.GetSandboxConnectionInfo(session.RunID, session.ScopedToken)
			if connErr == nil && (connInfo.Sandboxable || !connInfo.Polling.Completed) {
				status = "active"
			}

			if status == "expired" {
				expiredKeys = append(expiredKeys, key)
			} else {
				sandboxes = append(sandboxes, SandboxInfo{
					RunID:      session.RunID,
					Status:     status,
					ConfigFile: session.ConfigFile,
					Branch:     branch,
				})
			}
		}
	}

	if len(expiredKeys) > 0 {
		for _, key := range expiredKeys {
			delete(storage.Sandboxes, key)
		}
		storageChanged = true
	}

	if storageChanged {
		if err := storage.Save(); err != nil {
			fmt.Fprintf(s.Stderr, "Warning: failed to save sandbox storage: %v\n", err)
		}
	}

	if !cfg.Json {
		s.printSandboxList(sandboxes)
	}

	activeCount := 0
	for _, sb := range sandboxes {
		if sb.Status == "active" {
			activeCount++
		}
	}
	s.recordTelemetry("sandbox.list", map[string]any{
		"total_count":  len(sandboxes),
		"active_count": activeCount,
		"pruned_count": len(expiredKeys),
	})

	return &ListSandboxesResult{Sandboxes: sandboxes}, nil
}

func (s Service) printSandboxList(sandboxes []SandboxInfo) {
	if len(sandboxes) == 0 {
		fmt.Fprintln(s.Stdout, "No sandbox sessions found.")
	} else {
		fmt.Fprintf(s.Stdout, "%-40s %-10s %-25s %s\n", "RUN", "STATUS", "CONFIG", "BRANCH")
		for _, sb := range sandboxes {
			fmt.Fprintf(s.Stdout, "%-40s %-10s %-25s %s\n", sb.RunID, sb.Status, sb.ConfigFile, sb.Branch)
		}
	}
}

func (s Service) StopSandbox(cfg StopSandboxConfig) (*StopSandboxResult, error) {
	lockFile, lockErr := s.lockSandboxStorageWithInfo(cfg.Json)
	if lockErr != nil {
		return nil, errors.Wrap(lockErr, "unable to lock sandbox storage")
	}
	defer UnlockSandboxStorage(lockFile)

	storage, err := LoadSandboxStorage()
	if err != nil {
		return nil, errors.Wrap(err, "unable to load sandbox sessions")
	}

	var toStop []SandboxSession
	var keys []string

	if cfg.All {
		for key, session := range storage.AllSessions() {
			toStop = append(toStop, session)
			keys = append(keys, key)
		}
	} else if cfg.RunID != "" {
		session, key, found := storage.FindByRunID(cfg.RunID)
		if !found {
			return nil, fmt.Errorf("Sandbox with run ID '%s' not found in local storage.\nUse 'rwx sandbox list' to see available sandboxes.", cfg.RunID)
		}
		toStop = append(toStop, *session)
		keys = append(keys, key)
	} else {
		// Stop sandbox(es) for current CWD + branch
		cwd, err := os.Getwd()
		if err != nil {
			return nil, errors.Wrap(err, "unable to get current directory")
		}
		branch := GetCurrentGitBranch(cwd)

		sessions := storage.GetSessionsForBranch(branch)
		if len(sessions) == 0 && IsDetachedBranch(branch) {
			gitClient := &git.Client{Binary: "git", Dir: cwd}
			sessions = storage.GetSessionsForBranchByAncestry(branch, gitClient)
			if len(sessions) > 0 {
				_ = storage.Save()
			}
		}
		if len(sessions) == 0 {
			return nil, fmt.Errorf("No sandbox found for branch %s.\nUse 'rwx sandbox list' to see available sandboxes, or use --id to specify a run ID.", branch)
		}
		for _, session := range sessions {
			toStop = append(toStop, session)
			keys = append(keys, SessionKey(branch, session.ConfigFile))
		}
	}

	stopped := make([]StoppedSandbox, 0, len(toStop))

	now := time.Now().UTC()
	for i, session := range toStop {
		wasRunning := false
		cancelMethod := ""

		// Check if sandbox is still active and send stop command (use scoped token if available)
		connInfo, err := s.APIClient.GetSandboxConnectionInfo(session.RunID, session.ScopedToken)
		if err == nil && connInfo.Sandboxable {
			if err := s.connectSSH(&connInfo); err == nil {
				_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_end__")
				s.SSHClient.Close()
				wasRunning = true
				cancelMethod = "ssh"
			} else {
				// SSH connection failed — cancel via API to avoid orphaned runs
				if cancelErr := s.APIClient.CancelRun(session.RunID, session.ScopedToken); cancelErr != nil {
					fmt.Fprintf(s.Stderr, "Warning: failed to cancel run %s: %v\n", session.RunID, cancelErr)
				}
				wasRunning = true
				cancelMethod = "api"
			}
		} else if err == nil && !connInfo.Polling.Completed {
			// Run is still active but not yet sandboxable — cancel it server-side
			if cancelErr := s.APIClient.CancelRun(session.RunID, session.ScopedToken); cancelErr != nil {
				fmt.Fprintf(s.Stderr, "Warning: failed to cancel run %s: %v\n", session.RunID, cancelErr)
			}
			wasRunning = true
			cancelMethod = "api"
		}

		// Wait for the server to confirm the run has completed so that
		// immediately-following commands (exec, list, etc.) see consistent state.
		if wasRunning {
			s.waitForSandboxCompletion(session.RunID, session.ScopedToken)
		}

		// Remove from storage
		delete(storage.Sandboxes, keys[i])

		if !cfg.Json {
			if wasRunning {
				fmt.Fprintf(s.Stdout, "Stopped sandbox: %s\n", session.RunID)
			} else {
				fmt.Fprintf(s.Stdout, "Sandbox already stopped: %s\n", session.RunID)
			}
		}

		var lifetimeS int64
		if session.CreatedAt != nil {
			lifetimeS = int64(now.Sub(*session.CreatedAt).Seconds())
		}
		s.recordTelemetry("sandbox.stop", map[string]any{
			"lifetime_s":    lifetimeS,
			"exec_count":    session.ExecCount,
			"cancel_method": cancelMethod,
		})

		stopped = append(stopped, StoppedSandbox{
			RunID:      session.RunID,
			WasRunning: wasRunning,
		})
	}

	if err := storage.Save(); err != nil {
		fmt.Fprintf(s.Stderr, "Warning: Unable to save sandbox sessions: %v\n", err)
	}

	return &StopSandboxResult{Stopped: stopped}, nil
}

// waitForSandboxCompletion polls the server until the run is confirmed
// completed. This prevents immediately-following commands from finding
// a stale in-progress run during the window between stop signal and
// server-side teardown.
func (s Service) waitForSandboxCompletion(runID, scopedToken string) {
	const (
		timeout         = 30 * time.Second
		defaultInterval = 500 * time.Millisecond
	)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		connInfo, err := s.APIClient.GetSandboxConnectionInfo(runID, scopedToken)
		if err != nil {
			// Error (404, 410, etc.) means the run is no longer active
			return
		}
		// Polling.Completed=true with Sandboxable=true means the sandbox is
		// ready, not ended; keep waiting until it transitions to Sandboxable=false.
		if connInfo.Polling.Completed && !connInfo.Sandboxable {
			return
		}

		interval := defaultInterval
		if connInfo.Polling.BackoffMs != nil && *connInfo.Polling.BackoffMs > 0 {
			interval = time.Duration(*connInfo.Polling.BackoffMs) * time.Millisecond
		}
		time.Sleep(interval)
	}

	fmt.Fprintf(s.Stderr, "Warning: timed out waiting for sandbox %s to fully stop\n", runID)
}

func (s Service) ResetSandbox(cfg ResetSandboxConfig) (*ResetSandboxResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get current directory")
	}
	branch := GetCurrentGitBranch(cwd)

	var oldRunID string
	cancelMethod := ""

	// Check for existing sandbox with same config file.
	// Lock around the load+delete+save to cooperate with concurrent writers.
	lockFile, lockErr := s.lockSandboxStorageWithInfo(cfg.Json)
	if lockErr != nil {
		fmt.Fprintf(s.Stderr, "Warning: Unable to lock sandbox storage: %v\n", lockErr)
	}

	storage, err := LoadSandboxStorage()
	if err != nil {
		fmt.Fprintf(s.Stderr, "Warning: Unable to load sandbox sessions: %v\n", err)
	} else {
		session, found := storage.GetSession(branch, cfg.ConfigFile)
		if !found && IsDetachedBranch(branch) {
			gitClient := &git.Client{Binary: "git", Dir: cwd}
			session, found = storage.GetSessionByAncestry(branch, cfg.ConfigFile, gitClient)
		}
		if found {
			oldRunID = session.RunID

			// Check if still running and stop it (use scoped token if available)
			connInfo, err := s.APIClient.GetSandboxConnectionInfo(session.RunID, session.ScopedToken)
			if err == nil && connInfo.Sandboxable {
				if err := s.connectSSH(&connInfo); err == nil {
					_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_end__")
					s.SSHClient.Close()
					cancelMethod = "ssh"
				} else {
					// SSH connection failed — cancel via API to avoid orphaned runs
					if cancelErr := s.APIClient.CancelRun(session.RunID, session.ScopedToken); cancelErr != nil {
						fmt.Fprintf(s.Stderr, "Warning: failed to cancel run %s: %v\n", session.RunID, cancelErr)
					}
					cancelMethod = "api"
				}
			} else if err == nil && !connInfo.Polling.Completed {
				// Run is still active but not yet sandboxable — cancel it server-side
				if cancelErr := s.APIClient.CancelRun(session.RunID, session.ScopedToken); cancelErr != nil {
					fmt.Fprintf(s.Stderr, "Warning: failed to cancel run %s: %v\n", session.RunID, cancelErr)
				}
				cancelMethod = "api"
			}

			// Wait for the server to confirm the old run has completed before
			// starting a new sandbox, avoiding a window where the old run is
			// still in-progress and could conflict with the new one.
			if cancelMethod != "" {
				s.waitForSandboxCompletion(session.RunID, session.ScopedToken)
			}

			// Remove old session
			storage.DeleteSession(branch, cfg.ConfigFile)
			if err := storage.Save(); err != nil {
				fmt.Fprintf(s.Stderr, "Warning: Unable to save sandbox sessions: %v\n", err)
			}

			if !cfg.Json {
				fmt.Fprintf(s.Stdout, "Stopped old sandbox: %s\n", oldRunID)
			}
		}
	}

	// Pass the lock to StartSandbox so no concurrent process can slip in
	// between the old session removal and the new session creation.
	startResult, err := s.StartSandbox(StartSandboxConfig{
		ConfigFile:     cfg.ConfigFile,
		RwxDirectory:   cfg.RwxDirectory,
		Json:           cfg.Json,
		Wait:           cfg.Wait,
		InitParameters: cfg.InitParameters,
		storageLock:    lockFile,
	})
	if err != nil {
		return nil, err
	}

	s.recordTelemetry("sandbox.reset", map[string]any{
		"cancel_method": cancelMethod,
	})

	return &ResetSandboxResult{
		OldRunID: oldRunID,
		NewRunID: startResult.RunID,
		RunURL:   startResult.RunURL,
	}, nil
}

// pullChangesFromSandbox pulls changes from the sandbox back to the local working directory.
// It assumes the SSH connection is already established.
func (s Service) pullChangesFromSandbox(cwd string, jsonMode bool) ([]string, int, error) {
	// Mark start of sync operations (Mint filters these from logs)
	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_start__")

	// Include untracked files in the diff by adding them with intent-to-add
	// Get untracked files, add with -N, get diff, then reset
	lsExitCode, untrackedOutput, lsErr := s.SSHClient.ExecuteCommandWithOutput("/usr/bin/git ls-files --others --exclude-standard")
	if lsErr != nil {
		_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
		return nil, 0, errors.Wrap(lsErr, "failed to list untracked files in sandbox")
	}
	if lsExitCode != 0 {
		_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
		return nil, 0, fmt.Errorf("failed to list untracked files in sandbox: git ls-files failed with exit code %d", lsExitCode)
	}

	untrackedFiles := []string{}
	for _, f := range strings.Split(strings.TrimSpace(untrackedOutput), "\n") {
		if f != "" {
			untrackedFiles = append(untrackedFiles, f)
		}
	}

	// Add untracked files with intent-to-add
	if len(untrackedFiles) > 0 {
		quotedFiles := make([]string, len(untrackedFiles))
		for i, f := range untrackedFiles {
			quotedFiles[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(f, "'", "'\\''"))
		}
		addCmd := fmt.Sprintf("/usr/bin/git add -N -- %s", strings.Join(quotedFiles, " "))
		addExitCode, _, addErr := s.SSHClient.ExecuteCommandWithOutput(addCmd)
		if addErr != nil {
			_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
			return nil, 0, errors.Wrap(addErr, "failed to stage untracked files in sandbox")
		}
		if addExitCode != 0 {
			_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
			return nil, 0, fmt.Errorf("failed to stage untracked files in sandbox: git add failed with exit code %d", addExitCode)
		}
	}

	// Get patch from sandbox (stdout only to avoid output capture issues after sync markers)
	exitCode, patch, err := s.SSHClient.ExecuteCommandWithOutput("/usr/bin/git diff refs/rwx-sync")
	patchBytes := len(patch)

	// Reset the intent-to-add for untracked files
	if len(untrackedFiles) > 0 {
		quotedFiles := make([]string, len(untrackedFiles))
		for i, f := range untrackedFiles {
			quotedFiles[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(f, "'", "'\\''"))
		}
		resetCmd := fmt.Sprintf("/usr/bin/git reset HEAD -- %s", strings.Join(quotedFiles, " "))
		resetExitCode, _, resetErr := s.SSHClient.ExecuteCommandWithOutput(resetCmd)
		if resetErr != nil {
			fmt.Fprintf(s.Stderr, "Warning: failed to reset staged files in sandbox: %v\n", resetErr)
		} else if resetExitCode != 0 {
			fmt.Fprintf(s.Stderr, "Warning: failed to reset staged files in sandbox (exit code %d)\n", resetExitCode)
		}
	}

	// Mark end of sync operations
	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
	if err != nil {
		return nil, patchBytes, errors.Wrap(err, "failed to get diff from sandbox")
	}
	if exitCode != 0 {
		return nil, patchBytes, fmt.Errorf("failed to get changes from sandbox: git diff failed with exit code %d", exitCode)
	}

	sandboxPatchFiles := parsePatchFiles(patch)

	if len(sandboxPatchFiles) == 0 {
		if !jsonMode {
			fmt.Fprintln(s.Stdout, "No changes to pull from sandbox.")
		}
		return []string{}, 0, nil
	}

	// Apply sandbox patch locally (git apply is atomic — on failure nothing is modified)
	if len(strings.TrimSpace(patch)) > 0 {
		cmd := s.GitClient.ApplyPatch([]byte(patch))
		if err := cmd.Run(); err != nil {
			// Save the full patch so it can be inspected or applied manually
			patchSavePath := saveRejectedPatch([]byte(patch))

			// Retry with --reject: applies hunks that succeed, writes .rej files for the rest
			rejectCmd := s.GitClient.ApplyPatchReject([]byte(patch))
			rejectOutput, rejectErr := rejectCmd.CombinedOutput()

			if rejectErr != nil {
				// Find which files got .rej files by checking the patch file list
				rejFiles := findRejFiles(cwd, sandboxPatchFiles)

				msg := "failed to apply patch locally"
				if len(rejFiles) > 0 {
					msg = fmt.Sprintf("patch partially applied. %d file(s) have conflicts:\n", len(rejFiles))
					for _, f := range rejFiles {
						msg += fmt.Sprintf("  %s (see %s.rej)\n", f, f)
					}
					msg += "Resolve the conflicts in each .rej file, then delete the .rej files."
				} else if len(rejectOutput) > 0 {
					msg = fmt.Sprintf("failed to apply patch locally: %s", strings.TrimSpace(string(rejectOutput)))
				}
				if patchSavePath != "" {
					msg += fmt.Sprintf("\nFull patch saved to %s", patchSavePath)
				}
				return sandboxPatchFiles, patchBytes, errors.WrapSentinel(fmt.Errorf("%s", msg), errors.ErrPatch)
			}

			// --reject succeeded fully (all hunks applied despite initial failure)
			if patchSavePath != "" {
				_ = os.Remove(patchSavePath)
			}
		}
	}

	files := sandboxPatchFiles

	if !jsonMode {
		fmt.Fprintf(s.Stdout, "Pulled %d file(s) from sandbox:\n", len(files))
		for _, f := range files {
			fmt.Fprintf(s.Stdout, "  %s\n", f)
		}
	}

	return files, patchBytes, nil
}

func (s Service) cleanSandboxState() error {
	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_start__")

	exitCode, err := s.SSHClient.ExecuteCommand(
		"/usr/bin/git checkout . >/dev/null 2>&1; /usr/bin/git clean -fd >/dev/null 2>&1",
	)
	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
	if err != nil {
		return errors.Wrap(err, "failed to clean sandbox state")
	}
	if exitCode != 0 {
		return fmt.Errorf("failed to clean sandbox state (exit code %d)", exitCode)
	}

	return nil
}

// revertSandbox resets the sandbox working tree to a clean HEAD state.
// This runs after pull so the next exec starts from a known clean baseline.
func (s Service) revertSandbox() error {
	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_start__")

	exitCode, err := s.SSHClient.ExecuteCommand("/usr/bin/git checkout . >/dev/null 2>&1; /usr/bin/git clean -fd >/dev/null 2>&1; /usr/bin/git update-ref refs/rwx-sync HEAD 2>/dev/null")

	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")

	if err != nil {
		return errors.Wrap(err, "failed to revert sandbox")
	}
	if exitCode != 0 {
		return fmt.Errorf("failed to revert sandbox (exit code %d)", exitCode)
	}
	return nil
}

// parsePatchFiles extracts file paths from a git patch
func parsePatchFiles(patch string) []string {
	var files []string
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			// Format: diff --git a/path b/path
			parts := strings.Split(line, " ")
			if len(parts) >= 4 {
				file := strings.TrimPrefix(parts[3], "b/")
				files = append(files, file)
			}
		}
	}
	return files
}

// parseNewFilePaths returns the b/ paths from each diff block that has a
// `new file mode` header — i.e. additions only, not modifications or deletions.
func parseNewFilePaths(patch []byte) []string {
	var paths []string
	var current string
	for _, line := range strings.Split(string(patch), "\n") {
		if strings.HasPrefix(line, "diff --git") {
			current = ""
			parts := strings.Split(line, " ")
			if len(parts) >= 4 {
				current = strings.TrimPrefix(parts[3], "b/")
			}
		} else if strings.HasPrefix(line, "new file mode") && current != "" {
			paths = append(paths, current)
			current = ""
		}
	}
	return paths
}

// saveRejectedPatch writes the patch to .rwx/sandboxes/patch-rejected.diff for manual inspection.
func saveRejectedPatch(patch []byte) string {
	rwxDir, err := findRwxDirectoryPath("")
	if err != nil || rwxDir == "" {
		return ""
	}

	dir := filepath.Join(rwxDir, "sandboxes")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ""
	}

	path := filepath.Join(dir, "patch-rejected.diff")
	if err := os.WriteFile(path, patch, 0644); err != nil {
		return ""
	}

	return path
}

// findRejFiles checks which files from the patch have corresponding .rej files.
func findRejFiles(cwd string, patchFiles []string) []string {
	var rejFiles []string
	for _, f := range patchFiles {
		rejPath := filepath.Join(cwd, f+".rej")
		if _, err := os.Stat(rejPath); err == nil {
			rejFiles = append(rejFiles, f)
		}
	}
	return rejFiles
}

// findAllRejFiles walks the working directory looking for .rej files.
func findAllRejFiles(cwd string) []string {
	var rejFiles []string
	_ = filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".rej") {
			rejFiles = append(rejFiles, path)
		}
		return nil
	})
	return rejFiles
}

// sandboxRunURL returns the run URL from the session if available.
// Returns empty string when no server-provided URL is stored, since
// client-constructed URLs lack the org slug and would 404.
func (s Service) sandboxRunURL(session *SandboxSession) string {
	if session != nil && session.RunURL != "" {
		return session.RunURL
	}
	return ""
}

// lockSandboxStorageWithInfo acquires the sandbox storage file lock, showing
// a spinner to the user when another process already holds it.
func (s Service) lockSandboxStorageWithInfo(jsonMode bool) (*SandboxStorageLock, error) {
	lock, err := TryLockSandboxStorage()
	if err == nil {
		return lock, nil
	}

	// Lock is contended — show info while we wait for it
	var stopSpinner func()
	if !jsonMode {
		stopSpinner = Spin("Waiting for another sandbox operation to complete...", s.StderrIsTTY, s.Stderr)
	}

	lock, err = LockSandboxStorage()

	if stopSpinner != nil {
		stopSpinner()
	}

	if err != nil {
		return nil, err
	}

	return lock, nil
}

// Helper methods

func (s Service) waitForSandboxReadyWithToken(runID, scopedToken string, jsonMode bool) (*api.SandboxConnectionInfo, error) {
	// Check once before showing spinner - sandbox may already be ready
	connInfo, err := s.APIClient.GetSandboxConnectionInfo(runID, scopedToken)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get sandbox connection info")
	}

	if connInfo.Sandboxable {
		return &connInfo, nil
	}

	if connInfo.Polling.Completed || connInfo.FailureReason != "" {
		return nil, s.sandboxCompletedError(runID, connInfo)
	}

	// Sandbox not ready yet - start spinner and poll
	var stopSpinner func()
	if !jsonMode {
		stopSpinner = Spin("Waiting for sandbox to be ready...", s.StdoutIsTTY, s.Stdout)
		defer stopSpinner()
	}

	for {
		// Use backoff from server, or default to 2 seconds
		backoffMs := 2000
		if connInfo.Polling.BackoffMs != nil {
			backoffMs = *connInfo.Polling.BackoffMs
		}
		time.Sleep(time.Duration(backoffMs) * time.Millisecond)

		connInfo, err = s.APIClient.GetSandboxConnectionInfo(runID, scopedToken)
		if err != nil {
			return nil, errors.Wrap(err, "unable to get sandbox connection info")
		}

		if connInfo.Sandboxable {
			return &connInfo, nil
		}

		if connInfo.Polling.Completed || connInfo.FailureReason != "" {
			return nil, s.sandboxCompletedError(runID, connInfo)
		}
	}
}

// sandboxCompletedError prints the error header to stderr, then the failing
// run's prompt to stdout (so they appear in a sensible order under combined
// output), and returns a handled error so main.go doesn't re-print the header.
// The returned error still carries ErrSandboxSetupFailure for telemetry
// classification.
func (s Service) sandboxCompletedError(runID string, connInfo api.SandboxConnectionInfo) error {
	var msg error
	switch connInfo.FailureReason {
	case "timed_out":
		msg = fmt.Errorf("Sandbox run '%s' timed out before becoming ready", runID)
	case "cancelled":
		msg = fmt.Errorf("Sandbox run '%s' was cancelled before becoming ready", runID)
	case "failed":
		msg = fmt.Errorf("Sandbox run '%s' failed before becoming ready", runID)
	default:
		msg = fmt.Errorf("Sandbox run '%s' completed before becoming ready", runID)
	}

	fmt.Fprintf(s.Stderr, "Error: %s\n", msg)
	s.printSandboxRunPrompt(runID)

	return errors.WrapSentinel(errors.WrapSentinel(msg, HandledError), errors.ErrSandboxSetupFailure)
}

// printSandboxRunPrompt fetches and prints the run prompt to stdout.
// Polls /status first to ensure results are indexed before fetching the prompt.
// Best-effort: silently skipped if unavailable.
func (s Service) printSandboxRunPrompt(runID string) {
	_, _ = s.GetRunStatus(GetRunStatusConfig{
		RunID: runID,
		Wait:  true,
		Json:  true,
	})
	if prompt, err := s.APIClient.GetRunPrompt(runID); err == nil && prompt != "" {
		fmt.Fprintf(s.Stdout, "\n%s", prompt)
	}
}

func (s Service) connectSSH(connInfo *api.SandboxConnectionInfo) error {
	privateUserKey, err := ssh.ParsePrivateKey([]byte(connInfo.PrivateUserKey))
	if err != nil {
		return errors.Wrap(err, "unable to parse key material retrieved from Cloud API")
	}

	rawPublicHostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(connInfo.PublicHostKey))
	if err != nil {
		return errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	publicHostKey, err := ssh.ParsePublicKey(rawPublicHostKey.Marshal())
	if err != nil {
		return errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	sshConfig := ssh.ClientConfig{
		User:            "mint-cli",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(privateUserKey)},
		HostKeyCallback: ssh.FixedHostKey(publicHostKey),
	}

	connectStart := time.Now()
	if err = s.SSHClient.Connect(connInfo.Address, sshConfig); err != nil {
		s.recordTelemetry("ssh.connect", map[string]any{
			"duration_ms": time.Since(connectStart).Milliseconds(),
			"success":     false,
		})
		return errors.WrapSentinel(fmt.Errorf("unable to establish SSH connection to remote host: %w", err), errors.ErrSSH)
	}

	s.recordTelemetry("ssh.connect", map[string]any{
		"duration_ms": time.Since(connectStart).Milliseconds(),
		"success":     true,
	})

	return nil
}

// SandboxTitle constructs a descriptive title for sandbox runs using the
// project directory name, branch, and config file (if non-default).
func SandboxTitle(cwd, branch, configFile string) string {
	project := filepath.Base(cwd)

	displayBranch := branch
	if sha := DetachedShortSHA(branch); sha != "" {
		displayBranch = fmt.Sprintf("detached %s", sha)
	} else if branch == "" || branch == "detached" {
		displayBranch = "detached"
	}

	title := fmt.Sprintf("Sandbox: %s (%s)", project, displayBranch)

	// Include config file if it's not the default
	if configFile != "" && configFile != ".rwx/sandbox.yml" {
		title = fmt.Sprintf("%s [%s]", title, configFile)
	}

	return title
}

func (s Service) syncChangesToSandbox(jsonMode bool, baselineOnly bool) (int, error) {
	// Warn if .rej files from a previous failed pull are still present
	if cwd, err := os.Getwd(); err == nil {
		if rejFiles := findAllRejFiles(cwd); len(rejFiles) > 0 {
			fmt.Fprintf(s.Stderr, "Warning: %d unresolved .rej file(s) from a previous pull:\n", len(rejFiles))
			for _, f := range rejFiles {
				rel, _ := filepath.Rel(cwd, f)
				if rel == "" {
					rel = f
				}
				fmt.Fprintf(s.Stderr, "  %s\n", rel)
			}
			fmt.Fprintln(s.Stderr, "These will be synced to the sandbox and may cause issues. Resolve and delete them when possible.")
		}
	}

	syncStart := time.Now()
	patch, lfsFiles, err := s.GitClient.GeneratePatch(nil)
	if err != nil {
		return 0, errors.Wrap(err, "failed to generate patch")
	}

	patchBytes := len(patch)
	lfsSkippedCount := 0

	// Record telemetry for all code paths (early returns included)
	var syncPushErr error
	defer func() {
		s.recordTelemetry("sandbox.sync_push", map[string]any{
			"patch_bytes":       patchBytes,
			"duration_ms":       time.Since(syncStart).Milliseconds(),
			"lfs_skipped_count": lfsSkippedCount,
			"success":           syncPushErr == nil,
		})
	}()

	// Warn about LFS files
	if lfsFiles != nil && lfsFiles.Count > 0 {
		lfsSkippedCount = lfsFiles.Count
		if !jsonMode {
			fmt.Fprintf(s.Stderr, "Warning: %d LFS file(s) changed locally and cannot be synced.\n", lfsFiles.Count)
		}
		return patchBytes, nil
	}

	// Even with no local changes, ensure refs/rwx-sync exists so pull has a valid baseline
	if len(patch) == 0 {
		_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_start__")
		exitCode, err := s.SSHClient.ExecuteCommand("/usr/bin/git update-ref refs/rwx-sync HEAD")
		_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
		if err != nil {
			syncPushErr = errors.Wrap(err, "failed to create sync ref with no local changes")
			return 0, syncPushErr
		}
		if exitCode != 0 {
			syncPushErr = fmt.Errorf("failed to create sync ref with no local changes (exit code %d)", exitCode)
			return 0, syncPushErr
		}
		return 0, nil
	}

	// Mark start of sync operations (Mint filters these from logs)
	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_start__")

	// Check that .git directory exists on the sandbox
	exitCode, _ := s.SSHClient.ExecuteCommand("test -d .git")
	if exitCode != 0 {
		_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
		syncPushErr = errors.ErrSandboxNoGitDir
		return 0, syncPushErr
	}

	// Apply patch on remote (use full path since sandbox session may have minimal PATH).
	// On a new sandbox the patch is already applied to the working tree server-side, so we only
	// stage it to the index (--cached) to establish the refs/rwx-sync baseline without double-applying.
	applyCmd := "/usr/bin/git apply --allow-empty -"
	if baselineOnly {
		applyCmd = "/usr/bin/git apply --cached --allow-empty -"
	}
	exitCode, applyOutput, err := s.SSHClient.ExecuteCommandWithStdinAndCombinedOutput(applyCmd, bytes.NewReader(patch))

	// Mark end of sync operations
	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")

	if err != nil {
		syncPushErr = errors.WrapSentinel(errors.Wrap(err, "failed to apply patch on sandbox"), errors.ErrPatch)
		return patchBytes, syncPushErr
	}
	if exitCode == 127 {
		syncPushErr = fmt.Errorf("git is not installed in the sandbox. Add a task that installs git before the sandbox task")
		return patchBytes, syncPushErr
	}
	if exitCode != 0 {
		errMsg := strings.TrimSpace(applyOutput)
		if errMsg != "" {
			syncPushErr = errors.WrapSentinel(fmt.Errorf("failed to sync changes to sandbox: git apply failed: %s", errMsg), errors.ErrPatch)
			return patchBytes, syncPushErr
		}
		syncPushErr = errors.WrapSentinel(fmt.Errorf("failed to sync changes to sandbox: git apply failed with exit code %d", exitCode), errors.ErrPatch)
		return patchBytes, syncPushErr
	}

	// In baselineOnly mode, --cached only stages new-file additions in the index. The sandbox-creation
	// patch (from GeneratePatchFile) excludes untracked files, so the working tree never received them.
	// Without materializing them here, refs/rwx-sync ends up ahead of the WT, and pull's
	// `git diff refs/rwx-sync` reports them as deletions — wiping local untracked files.
	if baselineOnly {
		if newFilePaths := parseNewFilePaths(patch); len(newFilePaths) > 0 {
			_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_start__")
			quoted := make([]string, len(newFilePaths))
			for i, f := range newFilePaths {
				quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(f, "'", "'\\''"))
			}
			checkoutCmd := fmt.Sprintf("/usr/bin/git checkout-index --force -- %s", strings.Join(quoted, " "))
			checkoutExitCode, checkoutOutput, checkoutErr := s.SSHClient.ExecuteCommandWithOutput(checkoutCmd)
			_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
			if checkoutErr != nil {
				syncPushErr = errors.Wrap(checkoutErr, "failed to materialize new files in sandbox")
				return patchBytes, syncPushErr
			}
			if checkoutExitCode != 0 {
				errMsg := strings.TrimSpace(checkoutOutput)
				if errMsg != "" {
					syncPushErr = fmt.Errorf("failed to materialize new files in sandbox: %s", errMsg)
				} else {
					syncPushErr = fmt.Errorf("failed to materialize new files in sandbox (exit code %d)", checkoutExitCode)
				}
				return patchBytes, syncPushErr
			}
		}
	}

	// Snapshot the synced state as a detached ref so pull can diff against it (exec-only changes).
	// We delete the old ref, commit, save the new ref, then reset HEAD back so the user's branch
	// tip is unchanged during exec. The old ref is deleted here (not earlier) so that if sync fails
	// before this point, pull still has the previous baseline to diff against.
	// Wrap in sync markers so these internal git commands don't appear in task logs.
	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_start__")
	// On baseline-only the patch is already staged via --cached; skip git add -A to avoid
	// capturing unrelated working-tree state left by setup tasks.
	snapshotCmd := "/usr/bin/git update-ref -d refs/rwx-sync 2>/dev/null; /usr/bin/git add -A && /usr/bin/git -c user.name=rwx -c user.email=rwx commit --allow-empty -m rwx-sync >/dev/null 2>&1 && /usr/bin/git update-ref refs/rwx-sync HEAD"
	if baselineOnly {
		snapshotCmd = "/usr/bin/git update-ref -d refs/rwx-sync 2>/dev/null; /usr/bin/git -c user.name=rwx -c user.email=rwx commit --allow-empty -m rwx-sync >/dev/null 2>&1 && /usr/bin/git update-ref refs/rwx-sync HEAD"
	}
	snapshotExitCode, snapshotErr := s.SSHClient.ExecuteCommand(snapshotCmd)
	if snapshotErr != nil {
		_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
		syncPushErr = errors.Wrap(snapshotErr, "failed to create sync snapshot ref")
		return patchBytes, syncPushErr
	}
	if snapshotExitCode != 0 {
		_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
		syncPushErr = fmt.Errorf("failed to create sync snapshot ref (exit code %d)", snapshotExitCode)
		return patchBytes, syncPushErr
	}

	resetExitCode, resetErr := s.SSHClient.ExecuteCommand("/usr/bin/git reset HEAD~1 >/dev/null 2>&1")
	_, _ = s.SSHClient.ExecuteCommand("__rwx_sandbox_sync_end__")
	if resetErr != nil {
		syncPushErr = errors.Wrap(resetErr, "failed to reset HEAD after sync snapshot")
		return patchBytes, syncPushErr
	}
	if resetExitCode != 0 {
		syncPushErr = fmt.Errorf("failed to reset HEAD after sync snapshot (exit code %d)", resetExitCode)
		return patchBytes, syncPushErr
	}

	return patchBytes, nil
}
