package dial

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Both shader variants must at least compile; glslangValidator checks them
// against the GLSL 1.10 (desktop) and GLSL ES 1.00 specs.
func TestDialShaderSourcesCompile(t *testing.T) {
	validator, err := exec.LookPath("glslangValidator")
	if err != nil {
		t.Skip("glslangValidator not installed")
	}
	for name, src := range map[string]string{
		"desktop.frag": dialShaderPreludeGL + dialShaderBody,
		"es.frag":      dialShaderPreludeES + dialShaderBody,
	} {
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(validator, p).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
	}
}
