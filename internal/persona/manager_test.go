package persona

import (
	"testing"
)

func TestLoadQAEngineerPersona(t *testing.T) {
	manager := NewManager("../../personas/examples")
	
	persona, err := manager.LoadPersona("qa-engineer")
	if err != nil {
		t.Fatalf("Failed to load qa-engineer persona: %v", err)
	}
	
	if persona.Name != "qa-engineer" {
		t.Errorf("Expected name 'qa-engineer', got '%s'", persona.Name)
	}
	
	if len(persona.FocusAreas) == 0 {
		t.Error("Expected focus areas to be populated")
	}
	
	if persona.Mission == "" {
		t.Error("Expected mission to be populated")
	}
}

func TestLoadProjectManagerPersona(t *testing.T) {
	manager := NewManager("../../personas/examples")
	
	persona, err := manager.LoadPersona("project-manager")
	if err != nil {
		t.Fatalf("Failed to load project-manager persona: %v", err)
	}
	
	if persona.Name != "project-manager" {
		t.Errorf("Expected name 'project-manager', got '%s'", persona.Name)
	}
	
	if len(persona.FocusAreas) == 0 {
		t.Error("Expected focus areas to be populated")
	}
	
	if persona.Mission == "" {
		t.Error("Expected mission to be populated")
	}
}

func TestListPersonas(t *testing.T) {
	manager := NewManager("../../personas/examples")
	
	personas, err := manager.ListPersonas()
	if err != nil {
		t.Fatalf("Failed to list personas: %v", err)
	}
	
	// Should have at least the 5 personas we know about
	expectedPersonas := []string{"code-reviewer", "decision-maker", "housekeeping-bot", "qa-engineer", "project-manager"}
	
	for _, expected := range expectedPersonas {
		found := false
		for _, persona := range personas {
			if persona == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find persona '%s' in list", expected)
		}
	}
}
