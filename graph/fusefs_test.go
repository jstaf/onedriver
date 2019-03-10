package graph

// test that both mkdir and rmdir work, as well as the potentially failing
// mkdir->rmdir->mkdir chaing caused by a bad cache
/*
func TestMkdirRmdir(t *testing.T) {
	failOnErr(t, exec.Command("mkdir", filepath.Join(TestDir, "folder1")).Run())
	failOnErr(t, exec.Command("rmdir", filepath.Join(TestDir, "folder1")).Run())
	failOnErr(t, exec.Command("mkdir", filepath.Join(TestDir, "folder1")).Run())
}
*/
