// A bunch of "black box" filesystem integration tests that test the
// functionality of key syscalls and their implementation. If something fails
// here, the filesystem is not functional.
package graph

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
)

// does ls work and can we find the Documents/Pictures folders
func TestLs(t *testing.T) {
	t.Parallel()
	stdout, err := exec.Command("ls", "mount").Output()
	failOnErr(t, err)
	sout := string(stdout)
	if !strings.Contains(sout, "Documents") {
		t.Fatal("Could not find \"Documents\" folder.")
	}
	if !strings.Contains(sout, "Documents") {
		t.Fatal("Could not find \"Pictures\" folder.")
	}
}

// can touch create an empty file
func TestTouchCreate(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "empty")
	syscall.Umask(022) // otherwise tests fail if default umask is 002
	failOnErr(t, exec.Command("touch", fname).Run())
	st, err := os.Stat(fname)
	failOnErr(t, err)
	if st.Size() != 0 {
		t.Fatal("size was not 0")
	}
	if st.Mode() != 0644 {
		t.Fatalf("Mode of new file was not 644, got %o instead.\n", st.Mode())
	}
	if st.IsDir() {
		t.Fatal("New file detected as directory")
	}
}

// does the touch command update modification time properly?
func TestTouchUpdateTime(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "modtime")
	failOnErr(t, exec.Command("touch", fname).Run())
	st1, _ := os.Stat(fname)

	time.Sleep(2 * time.Second)

	failOnErr(t, exec.Command("touch", fname).Run())
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
	failOnErr(t, exec.Command("touch", fname).Run())
	failOnErr(t, os.Chmod(fname, 0777))
	st, _ := os.Stat(fname)
	if st.Mode() != 0777 {
		t.Fatalf("Mode of file was not 0777, got %o instead", st.Mode())
	}
}

// test that both mkdir and rmdir work, as well as the potentially failing
// mkdir->rmdir->mkdir chain that fails if the cache hangs on to an old copy
// after rmdir
func TestMkdirRmdir(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "folder1")
	failOnErr(t, exec.Command("mkdir", fname).Run())
	failOnErr(t, exec.Command("rmdir", fname).Run())
	failOnErr(t, exec.Command("mkdir", fname).Run())
}

// test that we can write to a file and read its contents back correctly
func TestReadWrite(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "write.txt")
	content := "my hands are typing words\n"
	failOnErr(t, ioutil.WriteFile(fname, []byte(content), 0644))
	read, err := ioutil.ReadFile(fname)
	failOnErr(t, err)
	if string(read) != content {
		t.Fatalf("File content was not correct - got: %s\nwanted: %s\n",
			string(read), content)
	}
}

// test that we can create a file and rename it
//TODO this can fail if a server-side rename undoes the second local rename
func TestRenameMove(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "rename.txt")
	dname := filepath.Join(TestDir, "new-destination-name.txt")
	failOnErr(t, ioutil.WriteFile(fname, []byte("hopefully renames work\n"), 0644))
	failOnErr(t, os.Rename(fname, dname))
	st, err := os.Stat(dname)
	failOnErr(t, err)
	if st == nil {
		t.Fatal("Renamed file does not exist")
	}

	os.Mkdir(filepath.Join(TestDir, "dest"), 0755)
	dname2 := filepath.Join(TestDir, "dest/even-newer-name.txt")
	failOnErr(t, os.Rename(dname, dname2))
	st, err = os.Stat(dname2)
	failOnErr(t, err)
	if st == nil {
		t.Fatal("Renamed file does not exist")
	}
}

// test that copies work as expected
func TestCopy(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "copy-start.txt")
	dname := filepath.Join(TestDir, "copy-end.txt")
	content := "and copies too!\n"
	failOnErr(t, ioutil.WriteFile(fname, []byte(content), 0644))
	failOnErr(t, exec.Command("cp", fname, dname).Run())

	read, err := ioutil.ReadFile(fname)
	failOnErr(t, err)
	if string(read) != content {
		t.Fatalf("File content was not correct\ngot: %s\nwanted: %s\n",
			string(read), content)
	}
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
	failOnErr(t, err)
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
	failOnErr(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var counter int
	for scanner.Scan() {
		counter++
		if scanner.Text() != "append" {
			t.Fatalf("File text was wrong. Got \"%s\", wanted \"append\"\n", scanner.Text())
		}
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
	failOnErr(t, ioutil.WriteFile(fname, []byte(content), 0644))

	file, _ := os.OpenFile(fname, os.O_RDWR, 0644)
	defer file.Close()
	match := "my hands are typing words. aaaaaaa"

	n, err := file.WriteAt([]byte(match), 123)
	failOnErr(t, err)
	if n != len(match) {
		t.Fatalf("Got %d bytes written, wanted %d bytes.\n", n, len(match))
	}

	result := make([]byte, len(match))
	n, err = file.ReadAt(result, 123)
	failOnErr(t, err)

	if n != len(match) {
		t.Fatalf("Got %d bytes read, wanted %d bytes.\n", n, len(match))
	}
	if string(result) != match {
		t.Fatalf("Content did not match expected output.\n"+
			"Got: \"%s\"\n Wanted: \"%s\"\n",
			string(result), match)
	}
}

// Statfs should succeed
func TestStatFs(t *testing.T) {
	t.Parallel()
	var st syscall.Statfs_t
	err := syscall.Statfs(TestDir, &st)
	failOnErr(t, err)

	if st.Blocks == 0 {
		t.Fatal("StatFs failed, got 0 blocks!")
	}
}

// does unlink work? (because apparently we weren't testing that before...)
func TestUnlink(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "unlink_tester")
	failOnErr(t, exec.Command("touch", fname).Run())
	failOnErr(t, os.Remove(fname))
	stdout, _ := exec.Command("ls", "mount").Output()
	if strings.Contains(string(stdout), "unlink_tester") {
		t.Fatalf("Deleting %s did not work.", fname)
	}
}

// copy large file inside onedrive mount, then verify that we can still
// access selected lines
func TestUploadSession(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "dmel.fa")
	failOnErr(t, exec.Command("cp", "dmel.fa", fname).Run())

	contents, err := ioutil.ReadFile(fname)
	failOnErr(t, err)

	header := ">X dna:chromosome chromosome:BDGP6.22:X:1:23542271:1 REF"
	if string(contents[:len(header)]) != header {
		t.Fatalf("Could not read FASTA header. Wanted \"%s\", got \"%s\"\n",
			header, string(contents[:len(header)]))
	}

	final := "AAATAAAATAC\n" // makes yucky test output, but is the final line
	match := string(contents[len(contents)-len(final):])
	if match != final {
		t.Fatalf("Could not read final line of FASTA. Wanted \"%s\", got \"%s\"\n",
			final, match)
	}

	st, _ := os.Stat(fname)
	if st.Size() == 0 {
		t.Fatal("File size cannot be 0.")
	}

	// poll endpoint to make sure it has a size greater than 0
	size := uint64(len(contents))
	for i := 0; i < 60; i++ {
		time.Sleep(time.Second)
		item, _ := GetItemPath("/onedriver_tests/dmel.fa", auth)
		if item != nil && item.Size() == size {
			return
		}
	}
	t.Fatalf("\nUpload session did not complete successfully!")
}

func TestIgnoredFiles(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, ".Trash-1000")
	_, err := os.Stat(fname)
	if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatal("Somehow we found a non-existent file.")
	}
}

// OneDrive is case-insensitive due to limitations imposed by Windows NTFS
// filesystem. Make sure we prevent users of normal systems from running into
// issues with OneDrive's case-insensitivity.
func TestNTFSIsABadFilesystem(t *testing.T) {
	t.Parallel()
	failOnErr(t, ioutil.WriteFile(filepath.Join(TestDir, "case-sensitive.txt"),
		[]byte("NTFS is bad"), 0644))
	failOnErr(t, ioutil.WriteFile(filepath.Join(TestDir, "CASE-SENSITIVE.txt"),
		[]byte("yep"), 0644))

	content, err := ioutil.ReadFile(filepath.Join(TestDir, "Case-Sensitive.TXT"))
	failOnErr(t, err)
	if string(content) != "yep" {
		t.Fatalf("Did not find expected output. got: \"%s\", wanted \"%s\"\n",
			string(content), "yep")
	}
}

// same as last test, but with exclusive create() calls.
func TestNTFSIsABadFilesystem2(t *testing.T) {
	t.Parallel()
	file, err := os.OpenFile(filepath.Join(TestDir, "case-sensitive2.txt"), os.O_CREATE|os.O_EXCL, 0644)
	file.Close()
	failOnErr(t, err)

	file, err = os.OpenFile(filepath.Join(TestDir, "CASE-SENSITIVE2.txt"), os.O_CREATE|os.O_EXCL, 0644)
	file.Close()
	if err == nil {
		t.Fatal("We should be throwing an error, since OneDrive is case-insensitive.")
	}
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
	failOnErr(t, ioutil.WriteFile(secondName, []byte("new"), 0644))
	failOnErr(t, os.Rename(secondName, fname))
	contents, err := ioutil.ReadFile(fname)
	failOnErr(t, err)
	if string(contents) != "new" {
		t.Fatalf("Contents did not match expected output: got \"%s\", wanted \"new\"\n",
			string(contents))
	}

	// should fail
	thirdName := filepath.Join(TestDir, "new_name2.txt")
	failOnErr(t, ioutil.WriteFile(thirdName, []byte("this rename should work"), 0644))
	err = os.Rename(thirdName, filepath.Join(TestDir, "original_name.txt"))
	if err != nil {
		t.Fatal("Rename failed.")
	}

	_, err = os.Stat(fname)
	if err != nil {
		t.Fatalf("\"%s\" does not exist after the rename\n", fname)
	}
}

// This test is insurance to prevent tests (and the fs) from accidentally not
// storing case for filenames at all
func TestChildrenAreCasedProperly(t *testing.T) {
	t.Parallel()
	failOnErr(t, ioutil.WriteFile(
		filepath.Join(TestDir, "CASE-check.txt"), []byte("yep"), 0644))
	stdout, err := exec.Command("ls", TestDir).Output()
	failOnErr(t, err)
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
	if err != nil {
		t.Log(string(out))
		t.Fatal(err)
	}
	content, err := ioutil.ReadFile(fname)
	failOnErr(t, err)
	if !bytes.Contains(content, []byte("bagels")) {
		t.Fatalf("Populating a file via 'echo' failed. Got: \"%s\", wanted \"bagels\"\n", content)
	}
}

// Test that if we stat a file, we get some correct information back
func TestStat(t *testing.T) {
	t.Parallel()
	stat, err := os.Stat("mount/Documents")
	failOnErr(t, err)
	if stat.Name() != "Documents" {
		t.Fatalf("Name was not \"Documents\", got \"%s\" instead.\n", stat.Name())
	}

	if stat.ModTime().Year() < 1971 {
		t.Fatal("Modification time of /Documents wrong, got: " + stat.ModTime().String())
	}

	if !stat.IsDir() {
		t.Fatal("Mode of /Documents wrong, not detected as directory, got: " + string(stat.Mode()))
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
	failOnErr(t, ioutil.WriteFile(fname, []byte("i should be trashed"), 0644))

	out, err := exec.Command("gio", "trash", fname).CombinedOutput()
	failOnErr(t, err)
	if strings.Contains(string(out), "Unable to find or create trash directory") {
		t.Fatal(out)
	}
}
