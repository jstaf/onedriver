#!/bin/bash
set -eu

TESTDIR=mount/test
mkdir -p $TESTDIR
cd $TESTDIR

ls -la

# test creating and removing folders multiple times/cache persistence
mkdir folder1
rmdir folder1
mkdir folder1

# test creating files via touch
touch testfile

# test unlink
rm testfile

# test various types of writes
echo some content > write.txt
echo more content >> write.txt
wc -l write.txt

# test truncates
echo new stuff > write.txt
cat write.txt
ls -l

# test moves
mv write.txt move.txt
mv move.txt folder1
ls -l folder1/

# copy!
mkdir folder2
cp folder1/move.txt folder2/
ls -l folder2/

# move into and out of the fs
mv folder1/move.txt ../../
mv ../../move.txt .
ls -l

# get out of the fs and unmount
cd ../..
rm -rf mount/test
killall --signal SIGINT onedriver
