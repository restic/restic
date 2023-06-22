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
docker run --rm --hostname my-host -ti \
    -v $HOME/.restic/passfile:/pass \
    -v $HOME/importantdirectory:/data \
    -e RESTIC_REPOSITORY=rest:https://user:pass@hostname/ \
    restic/restic -p /pass backup /data
```

Restic relies on the hostname for various operations. Make sure to set a static
hostname using `--hostname` when creating a Docker container, otherwise Docker
will assign a random hostname each time.
