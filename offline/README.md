This package exists purely for the convenience of easily running tests which 
test the offline functionality of the graph package.

`unshare -nr` is used to deny network access, and then the tests are run using
cached data from the tests in the graph package.
