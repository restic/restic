************
REST Backend
************

Restic can interact with HTTP Backend that respects the following REST
API.

The following values are valid for ``{type}``:

* ``data``
* ``keys``
* ``locks``
* ``snapshots``
* ``index``
* ``config``

The API version is selected via the ``Accept`` HTTP header in the request. The
following values are defined:

* ``application/vnd.x.restic.rest.v1`` or empty: Select API version 1
* ``application/vnd.x.restic.rest.v2``: Select API version 2

The server will respond with the value of the highest version it supports in
the ``Content-Type`` HTTP response header for the HTTP requests which should
return JSON. Any different value for this header means API version 1.

The placeholder ``{path}`` in this document is a path to the repository, so
that multiple different repositories can be accessed. The default path is
``/``. The path must end with a slash.

POST {path}?create=true
=======================

This request is used to initially create a new repository. The server
responds with "200 OK" if the repository structure was created
successfully or already exists, otherwise an error is returned.

DELETE {path}
=============

Deletes the repository on the server side. The server responds with "200
OK" if the repository was successfully removed. If this function is not
implemented the server returns "501 Not Implemented", if this it is
denied by the server it returns "403 Forbidden".

HEAD {path}/config
==================

Returns "200 OK" if the repository has a configuration, an HTTP error
otherwise.

GET {path}/config
=================

Returns the content of the configuration file if the repository has a
configuration, an HTTP error otherwise.

Response format: binary/octet-stream

POST {path}/config
==================

Returns "200 OK" if the configuration of the request body has been
saved, an HTTP error otherwise.

GET {path}/{type}/
==================

API version 1
-------------

Returns a JSON array containing the names of all the blobs stored for a given
type, example:

.. code:: json

    [
      "245bc4c430d393f74fbe7b13325e30dbde9fb0745e50caad57c446c93d20096b",
      "85b420239efa1132c41cea0065452a40ebc20c6f8e0b132a5b2f5848360973ec",
      "8e2006bb5931a520f3c7009fe278d1ebb87eb72c3ff92a50c30e90f1b8cf3e60",
      "e75c8c407ea31ba399ab4109f28dd18c4c68303d8d86cc275432820c42ce3649"
    ]

API version 2
-------------

Returns a JSON array containing an object for each file of the given type. The
objects have two keys: ``name`` for the file name, and ``size`` for the size in
bytes.

.. code:: json

    [
      {
        "name": "245bc4c430d393f74fbe7b13325e30dbde9fb0745e50caad57c446c93d20096b",
        "size": 2341058
      },
      {
        "name": "85b420239efa1132c41cea0065452a40ebc20c6f8e0b132a5b2f5848360973ec",
        "size": 2908900
      },
      {
        "name": "8e2006bb5931a520f3c7009fe278d1ebb87eb72c3ff92a50c30e90f1b8cf3e60",
        "size": 3030712
      },
      {
        "name": "e75c8c407ea31ba399ab4109f28dd18c4c68303d8d86cc275432820c42ce3649",
        "size": 2804
      }
    ]

HEAD {path}/{type}/{name}
=========================

Returns "200 OK" if the blob with the given name and type is stored in
the repository, "404 not found" otherwise. If the blob exists, the HTTP
header ``Content-Length`` is set to the file size.

GET {path}/{type}/{name}
========================

Returns the content of the blob with the given name and type if it is
stored in the repository, "404 not found" otherwise.

If the request specifies a partial read with a Range header field, then
the status code of the response is 206 instead of 200 and the response
only contains the specified range.

Response format: binary/octet-stream

POST {path}/{type}/{name}
=========================

Saves the content of the request body as a blob with the given name and
type, an HTTP error otherwise.

Request format: binary/octet-stream

DELETE {path}/{type}/{name}
===========================

Returns "200 OK" if the blob with the given name and type has been
deleted from the repository, an HTTP error otherwise.


