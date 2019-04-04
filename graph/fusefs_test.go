package graph

import (
	"bufio"
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
	fname := filepath.Join(TestDir, "folder1")
	failOnErr(t, exec.Command("mkdir", fname).Run())
	failOnErr(t, exec.Command("rmdir", fname).Run())
	failOnErr(t, exec.Command("mkdir", fname).Run())
}

// test that we can write to a file and read its contents back correctly
func TestReadWrite(t *testing.T) {
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
func TestRenameMove(t *testing.T) {
	fname := filepath.Join(TestDir, "rename.txt")
	dname := filepath.Join(TestDir, "new-name.txt")
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
		if scanner.Text() != "append" {
			t.Fatalf("File text was wrong. Got \"%s\", wanted \"append\"\n", scanner.Text())
		}
	}
	if counter != 5 {
		t.Fatalf("Got wrong number of lines (%d), expected 5\n", counter)
	}
}

// identical to TestAppend, but truncates the file each time it is written to
func TestTruncate(t *testing.T) {
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
	var st syscall.Statfs_t
	err := syscall.Statfs(TestDir, &st)
	failOnErr(t, err)

	if st.Blocks == 0 {
		t.Fatal("StatFs failed, got 0 blocks!")
	}
}

// does unlink work? (because apparently we weren't testing that before...)
func TestUnlink(t *testing.T) {
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
	fname := "dmel.fa"
	dname := filepath.Join(TestDir, "dmel.fa")
	failOnErr(t, exec.Command("cp", fname, dname).Run())

	contents, err := ioutil.ReadFile(dname)
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

	st, _ := os.Stat(dname)
	if st.Size() == 0 {
		t.Fatal("File size cannot be 0.")
	}
}

func TestIgnoredFiles(t *testing.T) {
	fname := filepath.Join(TestDir, ".Trash-1000")
	_, err := os.Stat(fname)
	if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatal("Somehow we found a non-existent file.")
	}
}
