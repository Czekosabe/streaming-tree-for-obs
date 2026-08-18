package webassets

import "testing"

// TestFrontendAndLegalAreRootedCorrectly proves fs.Sub actually stripped the
// "embedded"/"legal" prefix: the placeholder file must be reachable at its
// own bare name, not at "embedded/.gitkeep"/"legal/.gitkeep".
func TestFrontendAndLegalAreRootedCorrectly(t *testing.T) {
	if _, err := Frontend().Open(".gitkeep"); err != nil {
		t.Errorf("Frontend() is not rooted at the embedded directory's own files: %v", err)
	}
	if _, err := Legal().Open(".gitkeep"); err != nil {
		t.Errorf("Legal() is not rooted at the legal directory's own files: %v", err)
	}
}
