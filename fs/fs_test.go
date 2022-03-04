// A bunch of "black box" filesystem integration tests that test the
// functionality of key syscalls and their implementation. If something fails
// here, the filesystem is not functional.
package fs

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Does Go's internal ReadDir function work? This is mostly here to compare against
// the offline versions of this test.
func TestReaddir(t *testing.T) {
	t.Parallel()
	files, err := ioutil.ReadDir("mount")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if file.Name() == "Documents" {
			return
		}
	}
	t.Fatal("Could not find \"Documents\" folder.")
}

// does ls work and can we find the Documents folder?
func TestLs(t *testing.T) {
	t.Parallel()
	stdout, err := exec.Command("ls", "mount").Output()
	require.NoError(t, err)
	sout := string(stdout)
	if !strings.Contains(sout, "Documents") {
		t.Fatal("Could not find \"Documents\" folder.")
	}
}

// can touch create an empty file?
func TestTouchCreate(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "empty")
	syscall.Umask(022) // otherwise tests fail if default umask is 002
	require.NoError(t, exec.Command("touch", fname).Run())
	st, err := os.Stat(fname)
	require.NoError(t, err)

	require.Zero(t, st.Size(), "Size should be zero.")
	if st.Mode() != 0644 {
		t.Fatal("Mode of new file was not 644, got", Octal(uint32(st.Mode())))
	}
	require.False(t, st.IsDir(), "New file detected as directory.")
}

// does the touch command update modification time properly?
func TestTouchUpdateTime(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "modtime")
	require.NoError(t, exec.Command("touch", fname).Run())
	st1, _ := os.Stat(fname)

	time.Sleep(2 * time.Second)

	require.NoError(t, exec.Command("touch", fname).Run())
	st2, _ := os.Stat(fname)

	if st2.ModTime().Equal(st1.ModTime()) || st2.ModTime().Before(st1.ModTime()) {
		t.Fatalf("File modification time was not updated by touch:\n"+
			"Before: %d\nAfter: %d\n", st1.ModTime().Unix(), st2.ModTime().Unix())
	}
}

// chmod should *just work*
func TestChmod(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "chmod_tester")
	require.NoError(t, exec.Command("touch", fname).Run())
	require.NoError(t, os.Chmod(fname, 0777))
	st, _ := os.Stat(fname)
	if st.Mode() != 0777 {
		t.Fatal("Mode of file was wrong, got", Octal(uint32(st.Mode())))
	}
}

// test that both mkdir and rmdir work, as well as the potentially failing
// mkdir->rmdir->mkdir chain that fails if the cache hangs on to an old copy
// after rmdir
func TestMkdirRmdir(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "folder1")
	require.NoError(t, os.Mkdir(fname, 0755))
	require.NoError(t, os.Remove(fname))
	require.NoError(t, os.Mkdir(fname, 0755))
}

// We shouldn't be able to rmdir nonempty directories
func TestRmdirNonempty(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(TestDir, "nonempty")
	require.NoError(t, os.Mkdir(dir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "contents"), 0755))

	require.Error(t, os.Remove(dir), "We somehow removed a nonempty directory!")

	require.NoError(t, os.RemoveAll(dir),
		"Could not remove a nonempty directory the correct way!")
}

// test that we can write to a file and read its contents back correctly
func TestReadWrite(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "write.txt")
	content := "my hands are typing words\n"
	require.NoError(t, ioutil.WriteFile(fname, []byte(content), 0644))
	read, err := ioutil.ReadFile(fname)
	require.NoError(t, err)
	assert.Equal(t, content, string(read), "File content was not correct.")
}

// ld can crash the filesystem because it starts writing output at byte 64 in previously
// empty file
func TestWriteOffset(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "main.c")
	require.NoError(t, ioutil.WriteFile(fname,
		[]byte(`#include <stdio.h>

int main(int argc, char **argv) {
	printf("ld writes files in a funny manner!");
}`), 0644))
	require.NoError(t, exec.Command("gcc", "-o", filepath.Join(TestDir, "main.o"), fname).Run())
}

// test that we can create a file and rename it
//TODO this can fail if a server-side rename undoes the second local rename
func TestRenameMove(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "rename.txt")
	dname := filepath.Join(TestDir, "new-destination-name.txt")
	require.NoError(t, ioutil.WriteFile(fname, []byte("hopefully renames work\n"), 0644))
	require.NoError(t, os.Rename(fname, dname))
	st, err := os.Stat(dname)
	require.NoError(t, err)
	require.NotNil(t, st, "Renamed file does not exist.")

	os.Mkdir(filepath.Join(TestDir, "dest"), 0755)
	dname2 := filepath.Join(TestDir, "dest/even-newer-name.txt")
	require.NoError(t, os.Rename(dname, dname2))
	st, err = os.Stat(dname2)
	require.NoError(t, err)
	require.NotNil(t, st, "Renamed file does not exist.")
}

// test that copies work as expected
func TestCopy(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "copy-start.txt")
	dname := filepath.Join(TestDir, "copy-end.txt")
	content := "and copies too!\n"
	require.NoError(t, ioutil.WriteFile(fname, []byte(content), 0644))
	require.NoError(t, exec.Command("cp", fname, dname).Run())

	read, err := ioutil.ReadFile(fname)
	require.NoError(t, err)
	assert.Equal(t, content, string(read), "File content was not correct.")
}

// do appends work correctly?
func TestAppend(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "append.txt")
	for i := 0; i < 5; i++ {
		file, _ := os.OpenFile(fname, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
		file.WriteString("append\n")
		file.Close()
	}

	file, err := os.Open(fname)
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var counter int
	for scanner.Scan() {
		counter++
		scanned := scanner.Text()
		if scanned != "append" {
			t.Fatalf("File text was wrong. Got \"%s\", wanted \"append\"\n", scanned)
		}
	}
	if counter != 5 {
		t.Fatalf("Got wrong number of lines (%d), expected 5\n", counter)
	}
}

// identical to TestAppend, but truncates the file each time it is written to
func TestTruncate(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "truncate.txt")
	for i := 0; i < 5; i++ {
		file, _ := os.OpenFile(fname, os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0644)
		file.WriteString("append\n")
		file.Close()
	}

	file, err := os.Open(fname)
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var counter int
	for scanner.Scan() {
		counter++
		assert.Equal(t, "append", scanner.Text(), "File text was wrong.")
	}
	if counter != 1 {
		t.Fatalf("Got wrong number of lines (%d), expected 1\n", counter)
	}
}

// can we seek to the middle of a file and do writes there correctly?
func TestReadWriteMidfile(t *testing.T) {
	t.Parallel()
	content := `Lorem ipsum dolor sit amet, consectetur adipiscing elit. 
Phasellus viverra dui vel velit eleifend, vel auctor nulla scelerisque.
Mauris volutpat a justo vel suscipit. Suspendisse diam lorem, imperdiet eget
fermentum ut, sodales a nunc. Phasellus eget mattis purus. Aenean vitae justo
condimentum, rutrum libero non, commodo ex. Nullam mi metus, accumsan sit
amet varius non, volutpat eget mi. Fusce sollicitudin arcu eget ipsum
gravida, ut blandit turpis facilisis. Quisque vel rhoncus nulla, ultrices
tempor turpis. Nullam urna leo, dapibus eu velit eu, venenatis aliquet
tortor. In tempus lacinia est, nec gravida ipsum viverra sed. In vel felis
vitae odio pulvinar egestas. Sed ullamcorper, nulla non molestie dictum,
massa lectus mattis dolor, in volutpat nulla lectus id neque.`
	fname := filepath.Join(TestDir, "midfile.txt")
	require.NoError(t, ioutil.WriteFile(fname, []byte(content), 0644))

	file, _ := os.OpenFile(fname, os.O_RDWR, 0644)
	defer file.Close()
	match := "my hands are typing words. aaaaaaa"

	n, err := file.WriteAt([]byte(match), 123)
	require.NoError(t, err)
	require.Equal(t, len(match), n, "Wrong number of bytes written.")

	result := make([]byte, len(match))
	n, err = file.ReadAt(result, 123)
	require.NoError(t, err)
	require.Equal(t, len(match), n, "Wrong number of bytes read.")

	require.Equal(t, match, string(result), "Content did not match expected output.")
}

// Statfs should succeed
func TestStatFs(t *testing.T) {
	t.Parallel()
	var st syscall.Statfs_t
	err := syscall.Statfs(TestDir, &st)
	require.NoError(t, err)
	require.NotZero(t, st.Blocks, "StatFs failed, got 0 blocks!")
}

// does unlink work? (because apparently we weren't testing that before...)
func TestUnlink(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "unlink_tester")
	require.NoError(t, exec.Command("touch", fname).Run())
	require.NoError(t, os.Remove(fname))
	stdout, _ := exec.Command("ls", "mount").Output()
	if strings.Contains(string(stdout), "unlink_tester") {
		t.Fatalf("Deleting %s did not work.", fname)
	}
}

// OneDrive is case-insensitive due to limitations imposed by Windows NTFS
// filesystem. Make sure we prevent users of normal systems from running into
// issues with OneDrive's case-insensitivity.
func TestNTFSIsABadFilesystem(t *testing.T) {
	t.Parallel()
	require.NoError(t, ioutil.WriteFile(filepath.Join(TestDir, "case-sensitive.txt"),
		[]byte("NTFS is bad"), 0644))
	require.NoError(t, ioutil.WriteFile(filepath.Join(TestDir, "CASE-SENSITIVE.txt"),
		[]byte("yep"), 0644))

	content, err := ioutil.ReadFile(filepath.Join(TestDir, "Case-Sensitive.TXT"))
	require.NoError(t, err)
	require.Equal(t, "yep", string(content), "Did not find expected output.")
}

// same as last test, but with exclusive create() calls.
func TestNTFSIsABadFilesystem2(t *testing.T) {
	t.Parallel()
	file, err := os.OpenFile(filepath.Join(TestDir, "case-sensitive2.txt"), os.O_CREATE|os.O_EXCL, 0644)
	file.Close()
	require.NoError(t, err)

	file, err = os.OpenFile(filepath.Join(TestDir, "CASE-SENSITIVE2.txt"), os.O_CREATE|os.O_EXCL, 0644)
	file.Close()
	require.Error(t, err,
		"We should be throwing an error, since OneDrive is case-insensitive.")
}

// Ensure that case-sensitivity collisions due to renames are handled properly
// (allow rename/overwrite for exact matches, deny when case-sensitivity would
// normally allow success)
func TestNTFSIsABadFilesystem3(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "original_NAME.txt")
	ioutil.WriteFile(fname, []byte("original"), 0644)

	// should work
	secondName := filepath.Join(TestDir, "new_name.txt")
	require.NoError(t, ioutil.WriteFile(secondName, []byte("new"), 0644))
	require.NoError(t, os.Rename(secondName, fname))
	contents, err := ioutil.ReadFile(fname)
	require.NoError(t, err)
	require.Equal(t, "new", string(contents), "Contents did not match expected output.")

	// should fail
	thirdName := filepath.Join(TestDir, "new_name2.txt")
	require.NoError(t, ioutil.WriteFile(thirdName, []byte("this rename should work"), 0644))
	err = os.Rename(thirdName, filepath.Join(TestDir, "original_name.txt"))
	require.NoError(t, err, "Rename failed.")

	_, err = os.Stat(fname)
	require.NoErrorf(t, err, "\"%s\" does not exist after the rename.", fname)
}

// This test is insurance to prevent tests (and the fs) from accidentally not
// storing case for filenames at all
func TestChildrenAreCasedProperly(t *testing.T) {
	t.Parallel()
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(TestDir, "CASE-check.txt"), []byte("yep"), 0644))
	stdout, err := exec.Command("ls", TestDir).Output()
	if err != nil {
		t.Fatalf("%s: %s", err, stdout)
	}
	if !strings.Contains(string(stdout), "CASE-check.txt") {
		t.Fatalf("Upper case filenames were not honored, "+
			"expected \"CASE-check.txt\" in output, got %s\n", string(stdout))
	}
}

// Test that when running "echo some text > file.txt" that file.txt actually
// becomes populated
func TestEchoWritesToFile(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "bagels")
	out, err := exec.Command("bash", "-c", "echo bagels > "+fname).CombinedOutput()
	require.NoError(t, err, out)

	content, err := ioutil.ReadFile(fname)
	require.NoError(t, err)
	if !bytes.Contains(content, []byte("bagels")) {
		t.Fatalf("Populating a file via 'echo' failed. Got: \"%s\", wanted \"bagels\"\n", content)
	}
}

// Test that if we stat a file, we get some correct information back
func TestStat(t *testing.T) {
	t.Parallel()
	stat, err := os.Stat("mount/Documents")
	require.NoError(t, err)
	require.Equal(t, "Documents", stat.Name(), "Name was not \"Documents\".")

	if stat.ModTime().Year() < 1971 {
		t.Fatal("Modification time of /Documents wrong, got: " + stat.ModTime().String())
	}

	if !stat.IsDir() {
		t.Fatalf("Mode of /Documents wrong, not detected as directory, got: %s", stat.Mode())
	}
}

// Question marks appear in `ls -l`s output if an item is populated via readdir,
// but subsequently not found by lookup. Also is a nice catch-all for fs
// metadata corruption, as `ls` will exit with 1 if something bad happens.
func TestNoQuestionMarks(t *testing.T) {
	t.Parallel()
	out, err := exec.Command("ls", "-l", "mount/").CombinedOutput()
	if strings.Contains(string(out), "??????????") || err != nil {
		t.Log("A Lookup() failed on an inode found by Readdir()")
		t.Log(string(out))
		t.FailNow()
	}
}

// Trashing items through nautilus or other Linux file managers is done via
// "gio trash". Make an item then trash it to verify that this works.
func TestGIOTrash(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "trash_me.txt")
	require.NoError(t, ioutil.WriteFile(fname, []byte("i should be trashed"), 0644))

	out, err := exec.Command("gio", "trash", fname).CombinedOutput()
	if err != nil {
		t.Log(string(out))
		t.Log(err)
		if st, err2 := os.Stat(fname); err2 == nil {
			if !st.IsDir() && strings.Contains(string(out), "Is a directory") {
				t.Skip("This is a GIO bug (it complains about test file being " +
					"a directory despite correct metadata from onedriver), skipping.")
			}
			t.Fatal(fname, "still exists after deletion!")
		}
	}
	if strings.Contains(string(out), "Unable to find or create trash directory") {
		t.Fatal(string(out))
	}
}

// Test that we are able to work around onedrive paging limits when
// listing a folder's children.
func TestListChildrenPaging(t *testing.T) {
	t.Parallel()
	// files have been prepopulated during test setup to avoid being picked up by
	// the delta thread
	items, err := graph.GetItemChildrenPath("/onedriver_tests/paging", auth)
	require.NoError(t, err)
	files, err := ioutil.ReadDir(filepath.Join(TestDir, "paging"))
	require.NoError(t, err)
	if len(files) < 201 {
		if len(items) < 201 {
			t.Logf("Skipping test, number of paging files from the API were also less than 201.\nAPI: %d\nFS: %d\n",
				len(items), len(files),
			)
			t.SkipNow()
		}
		t.Fatalf("Paging limit failed. Got %d files, wanted at least 201.\n", len(files))
	}
}

// Libreoffice writes to files in a funny manner and it can result in a 0 byte file
// being uploaded (can check syscalls via "inotifywait -m -r .").
func TestLibreOfficeSavePattern(t *testing.T) {
	t.Parallel()
	content := []byte("This will break things.")
	fname := filepath.Join(TestDir, "libreoffice.txt")
	require.NoError(t, ioutil.WriteFile(fname, content, 0644))

	out, err := exec.Command(
		"libreoffice",
		"--headless",
		"--convert-to", "docx",
		"--outdir", TestDir,
		fname,
	).CombinedOutput()
	require.NoError(t, err, out)

	assert.Eventually(t, func() bool {
		item, err := graph.GetItemPath("/onedriver_tests/libreoffice.docx", auth)
		if err == nil && item != nil {
			if item.Size == 0 {
				t.Fatal("Item size was 0!")
			}
			return true
		}
		return false
	}, retrySeconds, 3*time.Second,
		"Could not find /onedriver_tests/libreoffice.docx post-upload!",
	)
}
