REST Backend
============

Restic can interact with HTTP Backend that respects the following REST API. The
following values are valid for `{type}`: `data`, `keys`, `locks`, `snapshots`,
`index`, `config`. `{path}` is a path to the repository, so that multiple
different repositories can be accessed. The default path is `/`.

## HEAD {path}/config

Returns "200 OK" if the repository has a configuration,
an HTTP error otherwise.

## GET {path}/config

Returns the content of the configuration file if the repository has a configuration,
an HTTP error otherwise.

Response format: binary/octet-stream

## POST {path}/config

Returns "200 OK" if the configuration of the request body has been saved,
an HTTP error otherwise.

## GET {path}/{type}/

Returns a JSON array containing the names of all the blobs stored for a given type.

Response format: JSON

## HEAD {path}/{type}/{name}

Returns "200 OK" if the blob with the given name and type is stored in the repository,
"404 not found" otherwise. If the blob exists, the HTTP header `Content-Length`
is set to the file size.

## GET {path}/{type}/{name}

Returns the content of the blob with the given name and type if it is stored in the repository,
"404 not found" otherwise.

If the request specifies a partial read with a Range header field,
then the status code of the response is 206 instead of 200
and the response only contains the specified range.

Response format: binary/octet-stream

## POST {path}/{type}/{name}

Saves the content of the request body as a blob with the given name and type,
an HTTP error otherwise.

Request format: binary/octet-stream

## DELETE {path}/{type}/{name}

Returns "200 OK" if the blob with the given name and type has been deleted from the repository,
an HTTP error otherwise.
