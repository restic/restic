REST Backend
============

Restic can interact with HTTP Backend that respects the following REST API.

## HEAD /config

Returns "200 OK" if the repository has already been initialized, 
"404 repository not found" otherwise.

## GET /config

Returns the configuration if the repository has already been initialized, 
"404 repository not found" otherwise.

Response format: binary/octet-stream

## POST /config

Saves the configuration transmitted in the request body.
Returns "200 OK" if the configuration has been saved, 
"409 repository already initialized" if the repository has already been initialized.

Response format: text

## GET /{type}/

Returns a JSON array containing the IDs of all the blobs stored for a given type.

Response format: JSON

## HEAD /{type}/{blobID}

Returns "200 OK" if the repository contains a blob with the given ID and type,
"404 blob not found" otherwise.

## GET /{type}/{blobID}

Returns the content of the blob with the given ID and type,
"404 blob not found" otherwise.

Response format: binary/octet-stream

## POST /{type}/{blobID}

Saves the content of the request body in a blob with the given ID and type,
"409 blob already exists" if a blob has already been saved.

Request format: binary/octet-stream

## DELETE /{type}/{blobID}

Deletes the blob with the given ID and type.
Returns "200 OK" if the given blob exists and has been deleted,
"404 blob not found" otherwise.
