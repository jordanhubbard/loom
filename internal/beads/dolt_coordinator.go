package beads

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DoltInstance represents a running Dolt SQL server for a project
type DoltInstance struct {
	ProjectID string
	DataDir   string
	Port      int
	PID       int
	Cmd       *exec.Cmd
	Ready     bool
	StartedAt time.Time
	mu        sync.Mutex
}

// DoltCoordinator manages per-project Dolt servers with Loom as the master.
// Each project gets its own Dolt SQL server instance. Loom coordinates
// federation between them so all beads are visible system-wide while
// each project maintains ownership of its own beads.
type DoltCoordinator struct {
	mu           sync.RWMutex
	instances    map[string]*DoltInstance // projectID -> instance
	masterID     string                  // Project ID of the master (usually "loom-self")
	basePort     int                     // Starting port for Dolt instances
	nextPort     int                     // Next available port
	bdPath       string                  // Path to bd CLI
	shutdownOnce sync.Once
}

// NewDoltCoordinator creates a new coordinator for per-project Dolt servers.
// masterProjectID is the project that acts as the federation master.
// basePort is the starting port for allocating Dolt SQL server ports.
func NewDoltCoordinator(masterProjectID, bdPath string, basePort int) *DoltCoordinator {
	if basePort == 0 {
		basePort = 3307
	}
	return &DoltCoordinator{
		instances: make(map[string]*DoltInstance),
		masterID:  masterProjectID,
		basePort:  basePort,
		nextPort:  basePort,
		bdPath:    bdPath,
	}
}

// EnsureInstance ensures a Dolt SQL server is running for the given project.
// If already running, returns the existing instance. Otherwise, starts a new one.
func (dc *DoltCoordinator) EnsureInstance(ctx context.Context, projectID, dataDir string) (*DoltInstance, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Return existing instance if running
	if inst, ok := dc.instances[projectID]; ok && inst.Ready {
		return inst, nil
	}

	doltDir := filepath.Join(dataDir, "dolt")

	// Ensure the Dolt repo is initialized
	if err := dc.ensureDoltInit(doltDir); err != nil {
		return nil, fmt.Errorf("failed to initialize dolt for %s: %w", projectID, err)
	}

	// Allocate a port
	port := dc.allocatePort()

	// Start the Dolt SQL server
	inst, err := dc.startDoltServer(ctx, projectID, doltDir, port)
	if err != nil {
		return nil, fmt.Errorf("failed to start dolt server for %s: %w", projectID, err)
	}

	dc.instances[projectID] = inst

	// If this is not the master, set up federation with the master
	if projectID != dc.masterID {
		if masterInst, ok := dc.instances[dc.masterID]; ok && masterInst.Ready {
			go dc.setupFederation(projectID, inst, masterInst)
		}
	}

	return inst, nil
}

// GetInstance returns the Dolt instance for a project, if running.
func (dc *DoltCoordinator) GetInstance(projectID string) *DoltInstance {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.instances[projectID]
}

// GetMasterInstance returns the master Dolt instance.
func (dc *DoltCoordinator) GetMasterInstance() *DoltInstance {
	return dc.GetInstance(dc.masterID)
}

// ListInstances returns all running Dolt instances.
func (dc *DoltCoordinator) ListInstances() map[string]*DoltInstance {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	result := make(map[string]*DoltInstance, len(dc.instances))
	for k, v := range dc.instances {
		result[k] = v
	}
	return result
}

// StopInstance stops a specific project's Dolt server.
func (dc *DoltCoordinator) StopInstance(projectID string) error {
	dc.mu.Lock()
	inst, ok := dc.instances[projectID]
	if !ok {
		dc.mu.Unlock()
		return nil
	}
	delete(dc.instances, projectID)
	dc.mu.Unlock()

	return dc.stopDoltServer(inst)
}

// Shutdown stops all Dolt instances.
func (dc *DoltCoordinator) Shutdown() {
	dc.shutdownOnce.Do(func() {
		dc.mu.Lock()
		instances := make([]*DoltInstance, 0, len(dc.instances))
		for _, inst := range dc.instances {
			instances = append(instances, inst)
		}
		dc.instances = make(map[string]*DoltInstance)
		dc.mu.Unlock()

		for _, inst := range instances {
			if err := dc.stopDoltServer(inst); err != nil {
				log.Printf("[DoltCoordinator] Error stopping %s: %v", inst.ProjectID, err)
			}
		}
	})
}

// Status returns a summary of all Dolt instances.
func (dc *DoltCoordinator) Status() map[string]interface{} {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	instances := make([]map[string]interface{}, 0, len(dc.instances))
	for _, inst := range dc.instances {
		instances = append(instances, map[string]interface{}{
			"project_id": inst.ProjectID,
			"port":       inst.Port,
			"ready":      inst.Ready,
			"started_at": inst.StartedAt,
			"is_master":  inst.ProjectID == dc.masterID,
		})
	}

	return map[string]interface{}{
		"master_id":      dc.masterID,
		"instance_count": len(dc.instances),
		"instances":      instances,
	}
}

func (dc *DoltCoordinator) allocatePort() int {
	port := dc.nextPort
	dc.nextPort++
	return port
}

func (dc *DoltCoordinator) ensureDoltInit(doltDir string) error {
	if _, err := os.Stat(filepath.Join(doltDir, ".dolt")); err == nil {
		return nil // Already initialized
	}

	if err := os.MkdirAll(doltDir, 0755); err != nil {
		return err
	}

	cmd := exec.Command("dolt", "init", "--name", "loom", "--email", "loom@localhost")
	cmd.Dir = doltDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dolt init: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func (dc *DoltCoordinator) startDoltServer(ctx context.Context, projectID, doltDir string, port int) (*DoltInstance, error) {
	// Check if port is available
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("port %d not available: %w", port, err)
	}
	ln.Close()

	cmd := exec.Command("dolt", "sql-server",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
	)
	cmd.Dir = doltDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start dolt sql-server: %w", err)
	}

	inst := &DoltInstance{
		ProjectID: projectID,
		DataDir:   doltDir,
		Port:      port,
		PID:       cmd.Process.Pid,
		Cmd:       cmd,
		StartedAt: time.Now(),
	}

	// Wait for server to be ready (up to 30 seconds)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			inst.Ready = true
			log.Printf("[DoltCoordinator] Dolt server for %s ready on port %d (PID %d)", projectID, port, inst.PID)
			return inst, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Cleanup on timeout
	_ = cmd.Process.Kill()
	return nil, fmt.Errorf("dolt server for %s did not become ready within 30s", projectID)
}

func (dc *DoltCoordinator) stopDoltServer(inst *DoltInstance) error {
	if inst == nil || inst.Cmd == nil || inst.Cmd.Process == nil {
		return nil
	}

	inst.mu.Lock()
	defer inst.mu.Unlock()

	inst.Ready = false
	log.Printf("[DoltCoordinator] Stopping Dolt server for %s (PID %d)", inst.ProjectID, inst.PID)

	if err := inst.Cmd.Process.Signal(os.Interrupt); err != nil {
		_ = inst.Cmd.Process.Kill()
	}

	done := make(chan error, 1)
	go func() { done <- inst.Cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = inst.Cmd.Process.Kill()
	}

	return nil
}

// setupFederation adds a Dolt remote between a project instance and the master.
// This allows the master to pull beads from the project and vice versa.
func (dc *DoltCoordinator) setupFederation(projectID string, projectInst, masterInst *DoltInstance) {
	remoteName := fmt.Sprintf("loom-master")
	remoteURL := fmt.Sprintf("http://127.0.0.1:%d/beads", masterInst.Port)

	// Add the master as a remote in the project's Dolt instance
	cmd := exec.Command("dolt", "remote", "add", remoteName, remoteURL)
	cmd.Dir = projectInst.DataDir
	if out, err := cmd.CombinedOutput(); err != nil {
		outStr := strings.TrimSpace(string(out))
		if !strings.Contains(outStr, "already exists") {
			log.Printf("[DoltCoordinator] Failed to add federation remote for %s: %v: %s", projectID, err, outStr)
		}
	} else {
		log.Printf("[DoltCoordinator] Federation remote added: %s -> master (%s)", projectID, remoteURL)
	}

	// Add the project as a remote in the master's Dolt instance
	projectRemoteName := fmt.Sprintf("project-%s", projectID)
	projectRemoteURL := fmt.Sprintf("http://127.0.0.1:%d/beads", projectInst.Port)

	cmd = exec.Command("dolt", "remote", "add", projectRemoteName, projectRemoteURL)
	cmd.Dir = masterInst.DataDir
	if out, err := cmd.CombinedOutput(); err != nil {
		outStr := strings.TrimSpace(string(out))
		if !strings.Contains(outStr, "already exists") {
			log.Printf("[DoltCoordinator] Failed to add project remote to master for %s: %v: %s", projectID, err, outStr)
		}
	} else {
		log.Printf("[DoltCoordinator] Federation remote added: master -> %s (%s)", projectID, projectRemoteURL)
	}
}

// SyncFederationAll pulls beads from all project instances into the master.
func (dc *DoltCoordinator) SyncFederationAll(ctx context.Context) error {
	dc.mu.RLock()
	masterInst := dc.instances[dc.masterID]
	instances := make(map[string]*DoltInstance, len(dc.instances))
	for k, v := range dc.instances {
		instances[k] = v
	}
	dc.mu.RUnlock()

	if masterInst == nil || !masterInst.Ready {
		return fmt.Errorf("master instance not ready")
	}

	var lastErr error
	for projectID, inst := range instances {
		if projectID == dc.masterID || !inst.Ready {
			continue
		}

		remoteName := fmt.Sprintf("project-%s", projectID)
		cmd := exec.CommandContext(ctx, "dolt", "fetch", remoteName)
		cmd.Dir = masterInst.DataDir
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[DoltCoordinator] Federation fetch from %s failed: %v: %s", projectID, err, strings.TrimSpace(string(out)))
			lastErr = err
		} else {
			log.Printf("[DoltCoordinator] Federation fetch from %s succeeded", projectID)
		}
	}

	return lastErr
}

// SyncProject pulls beads from the master into a specific project instance.
func (dc *DoltCoordinator) SyncProject(ctx context.Context, projectID string) error {
	dc.mu.RLock()
	inst := dc.instances[projectID]
	masterInst := dc.instances[dc.masterID]
	dc.mu.RUnlock()

	if inst == nil || !inst.Ready {
		return fmt.Errorf("project %s instance not ready", projectID)
	}
	if masterInst == nil || !masterInst.Ready {
		return fmt.Errorf("master instance not ready")
	}

	cmd := exec.CommandContext(ctx, "dolt", "fetch", "loom-master")
	cmd.Dir = inst.DataDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("federation fetch from master failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// ConnectionString returns the MySQL-compatible connection string for a project's Dolt server.
func (inst *DoltInstance) ConnectionString() string {
	return fmt.Sprintf("root@tcp(127.0.0.1:%d)/beads", inst.Port)
}

// IsHealthy checks if the Dolt server is still responding.
func (inst *DoltInstance) IsHealthy() bool {
	if inst == nil || !inst.Ready {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", inst.Port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
