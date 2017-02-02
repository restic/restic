xattr
=====
Extended attribute support for Go (linux + darwin + freebsd).

"Extended attributes are name:value pairs associated permanently with files and directories, similar to the environment strings associated with a process. An attribute may be defined or undefined. If it is defined, its value may be empty or non-empty." [See more...](https://en.wikipedia.org/wiki/Extended_file_attributes)


### Example
```
  const path = "/tmp/myfile"
  const prefix = "user."

  if err = Setxattr(path, prefix+"test", []byte("test-attr-value")); err != nil {
    t.Fatal(err)
  }

  var data []byte
  data, err = Getxattr(path, prefix+"test"); err != nil {
    t.Fatal(err)
  }
```
