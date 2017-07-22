# Docker image

## Build

From the root of this repository run:

```
./docker/build.sh
```

image name will be `restic/restic:latest`

## Run

Set environment variable `RESTIC_REPOSITORY` and map volume to directories and
files like:

```
docker run --rm -ti \
    -v $HOME/.restic/passfile:/pass \
    -v $HOME/importantdirectory:/data \
    -e RESTIC_REPOSITORY=rest:https://user:pass@hostname/ \
    restic/restic -p /pass backup /data
```
