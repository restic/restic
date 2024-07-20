# Docker image

## Build

From the root of this repository run:

```
./docker/build.sh
```

image name will be `restic/restic:latest`

## Run
Set environment variable `RESTIC_REPOSITORY`, `RESTIC_PASSWORD` and map volume to directories and
files like:

```
docker run --rm --hostname my-host \
    -v $HOME/importantdirectory:/data \
    -e RESTIC_REPOSITORY=rest:https://user:pass@hostname/ \
    -e RESTIC_PASSWORD=/run/secrets/restic_password
    restic/restic
```

Restic relies on the hostname for various operations. Make sure to set a static
hostname using `--hostname` when creating a Docker container, otherwise Docker
will assign a random hostname each time.

## Advanced
Optionally you can change the source directory by setting `RESTIC_DATA` to an other path, 
defaults to /data.  
Or you can enable a [retention policy](https://restic.readthedocs.io/en/stable/060_forget.html#removing-snapshots-according-to-a-policy) 
by setting `RESTIC_FORGET`.
For other possible environment variables check: [Environment Variables](https://restic.readthedocs.io/en/stable/040_backup.html#environment-variables)

By removing the *--rm* tag, the container would not destroy itself after it is finished, 
only exits, and you can schedule your backups just by starting the container with cron like this:
```cron
0 0 * * * docker start restic
```

To not have your password plain text in your shell use a [docker-compose.yml](docker-compose.yml) and docker secrets.
