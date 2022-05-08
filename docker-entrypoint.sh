#!/bin/sh

#
# taken from https://github.com/c7ks7s/docker-entrypoint
#
set -eo pipefail

#
# populates an environment variable from a file useful with docker secrets
#
secretDebug()
{
  if [ ! -z "$ENV_SECRETS_DEBUG" ]; then
    echo -e "\033[1m$@\033[0m"
    echo
  fi
}

getSecrets () {
  for env_var in $(printenv | cut -f1 -d"=" | grep _FILE)
  do
    name="$env_var"
    eval value=\$$name

    if [ -f "$value" ]; then
      value=$(cat "${value}")
      export "${name%_FILE}"="$value"
      unset $name
      secretDebug "Expanded Secret! ${name%_FILE}=$value"
    else
      secretDebug "Secret file does not exist! $value"
    fi
  done
}

getSecrets

#
# End
#
exec "/usr/bin/restic"
