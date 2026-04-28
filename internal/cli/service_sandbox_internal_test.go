package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNewFilePaths(t *testing.T) {
	t.Run("returns paths only for new-file additions", func(t *testing.T) {
		patch := []byte(`diff --git a/tracked.txt b/tracked.txt
index abc..def 100644
--- a/tracked.txt
+++ b/tracked.txt
@@ -1 +1 @@
-old
+new
diff --git a/added.txt b/added.txt
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/added.txt
@@ -0,0 +1 @@
+content
diff --git a/removed.txt b/removed.txt
deleted file mode 100644
index abc1234..0000000
--- a/removed.txt
+++ /dev/null
@@ -1 +0,0 @@
-content
`)
		require.Equal(t, []string{"added.txt"}, parseNewFilePaths(patch))
	})

	t.Run("handles multiple new files", func(t *testing.T) {
		patch := []byte(`diff --git a/one.txt b/one.txt
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/one.txt
@@ -0,0 +1 @@
+one
diff --git a/two.txt b/two.txt
new file mode 100644
index 0000000..def5678
--- /dev/null
+++ b/two.txt
@@ -0,0 +1 @@
+two
`)
		require.Equal(t, []string{"one.txt", "two.txt"}, parseNewFilePaths(patch))
	})

	t.Run("handles nested paths", func(t *testing.T) {
		patch := []byte(`diff --git a/dir/sub/file.txt b/dir/sub/file.txt
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/dir/sub/file.txt
@@ -0,0 +1 @@
+content
`)
		require.Equal(t, []string{"dir/sub/file.txt"}, parseNewFilePaths(patch))
	})

	t.Run("returns nil for empty patch", func(t *testing.T) {
		require.Nil(t, parseNewFilePaths(nil))
		require.Nil(t, parseNewFilePaths([]byte("")))
	})

	t.Run("returns nil when no new files in patch", func(t *testing.T) {
		patch := []byte(`diff --git a/tracked.txt b/tracked.txt
index abc..def 100644
--- a/tracked.txt
+++ b/tracked.txt
@@ -1 +1 @@
-old
+new
`)
		require.Nil(t, parseNewFilePaths(patch))
	})
}
