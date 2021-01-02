#compdef _restic restic


function _restic {
  local -a commands

  _arguments -C \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '(-h --help)'{-h,--help}'[help for restic]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "backup:Create a new backup of files and/or directories"
      "cache:Operate on local cache directories"
      "cat:Print internal objects to stdout"
      "check:Check the repository for errors"
      "copy:Copy snapshots from one repository to another"
      "diff:Show differences between two snapshots"
      "dump:Print a backed-up file to stdout"
      "find:Find a file, a directory or restic IDs"
      "forget:Remove snapshots from the repository"
      "generate:Generate manual pages and auto-completion files (bash, zsh)"
      "help:Help about any command"
      "init:Initialize a new repository"
      "key:Manage keys (passwords)"
      "list:List objects in the repository"
      "ls:List files in a snapshot"
      "migrate:Apply migrations"
      "mount:Mount the repository"
      "prune:Remove unneeded data from the repository"
      "rebuild-index:Build a new index file"
      "recover:Recover data from the repository"
      "restore:Extract the data from a snapshot"
      "self-update:Update the restic binary"
      "snapshots:List all snapshots"
      "stats:Scan the repository and show basic statistics"
      "tag:Modify tags on snapshots"
      "unlock:Remove locks other processes created"
      "version:Print version information"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  backup)
    _restic_backup
    ;;
  cache)
    _restic_cache
    ;;
  cat)
    _restic_cat
    ;;
  check)
    _restic_check
    ;;
  copy)
    _restic_copy
    ;;
  diff)
    _restic_diff
    ;;
  dump)
    _restic_dump
    ;;
  find)
    _restic_find
    ;;
  forget)
    _restic_forget
    ;;
  generate)
    _restic_generate
    ;;
  help)
    _restic_help
    ;;
  init)
    _restic_init
    ;;
  key)
    _restic_key
    ;;
  list)
    _restic_list
    ;;
  ls)
    _restic_ls
    ;;
  migrate)
    _restic_migrate
    ;;
  mount)
    _restic_mount
    ;;
  prune)
    _restic_prune
    ;;
  rebuild-index)
    _restic_rebuild-index
    ;;
  recover)
    _restic_recover
    ;;
  restore)
    _restic_restore
    ;;
  self-update)
    _restic_self-update
    ;;
  snapshots)
    _restic_snapshots
    ;;
  stats)
    _restic_stats
    ;;
  tag)
    _restic_tag
    ;;
  unlock)
    _restic_unlock
    ;;
  version)
    _restic_version
    ;;
  esac
}

function _restic_backup {
  _arguments \
    '(*-e *--exclude)'{\*-e,\*--exclude}'[exclude a `pattern` (can be specified multiple times)]:' \
    '--exclude-caches[excludes cache directories that are marked with a CACHEDIR.TAG file. See https://bford.info/cachedir/ for the Cache Directory Tagging Standard]' \
    '*--exclude-file[read exclude patterns from a `file` (can be specified multiple times)]:' \
    '*--exclude-if-present[takes `filename[:header]`, exclude contents of directories containing filename (except filename itself) if header of that file is as provided (can be specified multiple times)]:' \
    '--exclude-larger-than[max `size` of the files to be backed up (allowed suffixes: k/K, m/M, g/G, t/T)]:' \
    '*--files-from[read the files to backup from `file` (can be combined with file args/can be specified multiple times)]:' \
    '(-f --force)'{-f,--force}'[force re-reading the target files/directories (overrides the "parent" flag)]' \
    '(-h --help)'{-h,--help}'[help for backup]' \
    '(-H --host)'{-H,--host}'[set the `hostname` for the snapshot manually. To prevent an expensive rescan use the "parent" flag]:' \
    '*--iexclude[same as --exclude `pattern` but ignores the casing of filenames]:' \
    '*--iexclude-file[same as --exclude-file but ignores casing of `file`names in patterns]:' \
    '--ignore-inode[ignore inode number changes when checking for modified files]' \
    '(-x --one-file-system)'{-x,--one-file-system}'[exclude other file systems]' \
    '--parent[use this parent `snapshot` (default: last snapshot in the repo that has the same target files/directories)]:' \
    '--stdin[read backup from stdin]' \
    '--stdin-filename[`filename` to use when reading from stdin]:' \
    '*--tag[add a `tag` for the new snapshot (can be specified multiple times)]:' \
    '--time[`time` of the backup (ex. '\''2012-11-01 22:08:41'\'') (default: now)]:' \
    '--with-atime[store the atime for all files and directories]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_cache {
  _arguments \
    '--cleanup[remove old cache directories]' \
    '(-h --help)'{-h,--help}'[help for cache]' \
    '--max-age[max age in `days` for cache directories to be considered old]:' \
    '--no-size[do not output the size of the cache directories]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_cat {
  _arguments \
    '(-h --help)'{-h,--help}'[help for cat]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_check {
  _arguments \
    '--check-unused[find unused blobs]' \
    '(-h --help)'{-h,--help}'[help for check]' \
    '--read-data[read all data blobs]' \
    '--read-data-subset[read subset n of m data packs (format: `n/m`)]:' \
    '--with-cache[use the cache]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_copy {
  _arguments \
    '(-h --help)'{-h,--help}'[help for copy]' \
    '(*-H *--host)'{\*-H,\*--host}'[only consider snapshots for this `host`, when no snapshot ID is given (can be specified multiple times)]:' \
    '--key-hint2[key ID of key to try decrypting the destination repository first (default: $RESTIC_KEY_HINT2)]:' \
    '--password-command2[shell `command` to obtain the destination repository password from (default: $RESTIC_PASSWORD_COMMAND2)]:' \
    '--password-file2[`file` to read the destination repository password from (default: $RESTIC_PASSWORD_FILE2)]:' \
    '*--path[only consider snapshots which include this (absolute) `path`, when no snapshot ID is given]:' \
    '--repo2[destination `repository` to copy snapshots to (default: $RESTIC_REPOSITORY2)]:' \
    '--tag[only consider snapshots which include this `taglist`, when no snapshot ID is given]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_diff {
  _arguments \
    '(-h --help)'{-h,--help}'[help for diff]' \
    '--metadata[print changes in metadata]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_dump {
  _arguments \
    '(-h --help)'{-h,--help}'[help for dump]' \
    '(*-H *--host)'{\*-H,\*--host}'[only consider snapshots for this host when the snapshot ID is "latest" (can be specified multiple times)]:' \
    '*--path[only consider snapshots which include this (absolute) `path` for snapshot ID "latest"]:' \
    '--tag[only consider snapshots which include this `taglist` for snapshot ID "latest"]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_find {
  _arguments \
    '--blob[pattern is a blob-ID]' \
    '(-h --help)'{-h,--help}'[help for find]' \
    '(*-H *--host)'{\*-H,\*--host}'[only consider snapshots for this `host`, when no snapshot ID is given (can be specified multiple times)]:' \
    '(-i --ignore-case)'{-i,--ignore-case}'[ignore case for pattern]' \
    '(-l --long)'{-l,--long}'[use a long listing format showing size and mode]' \
    '(-N --newest)'{-N,--newest}'[newest modification date/time]:' \
    '(-O --oldest)'{-O,--oldest}'[oldest modification date/time]:' \
    '--pack[pattern is a pack-ID]' \
    '*--path[only consider snapshots which include this (absolute) `path`, when no snapshot-ID is given]:' \
    '--show-pack-id[display the pack-ID the blobs belong to (with --blob or --tree)]' \
    '(*-s *--snapshot)'{\*-s,\*--snapshot}'[snapshot `id` to search in (can be given multiple times)]:' \
    '--tag[only consider snapshots which include this `taglist`, when no snapshot-ID is given]:' \
    '--tree[pattern is a tree-ID]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_forget {
  _arguments \
    '(-l --keep-last)'{-l,--keep-last}'[keep the last `n` snapshots]:' \
    '(-H --keep-hourly)'{-H,--keep-hourly}'[keep the last `n` hourly snapshots]:' \
    '(-d --keep-daily)'{-d,--keep-daily}'[keep the last `n` daily snapshots]:' \
    '(-w --keep-weekly)'{-w,--keep-weekly}'[keep the last `n` weekly snapshots]:' \
    '(-m --keep-monthly)'{-m,--keep-monthly}'[keep the last `n` monthly snapshots]:' \
    '(-y --keep-yearly)'{-y,--keep-yearly}'[keep the last `n` yearly snapshots]:' \
    '--keep-within[keep snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot]:' \
    '--keep-tag[keep snapshots with this `taglist` (can be specified multiple times)]:' \
    '*--host[only consider snapshots with the given `host` (can be specified multiple times)]:' \
    '--tag[only consider snapshots which include this `taglist` in the format `tag[,tag,...]` (can be specified multiple times)]:' \
    '*--path[only consider snapshots which include this (absolute) `path` (can be specified multiple times)]:' \
    '(-c --compact)'{-c,--compact}'[use compact output format]' \
    '(-g --group-by)'{-g,--group-by}'[string for grouping snapshots by host,paths,tags]:' \
    '(-n --dry-run)'{-n,--dry-run}'[do not delete anything, just print what would be done]' \
    '--prune[automatically run the '\''prune'\'' command if snapshots have been removed]' \
    '(-h --help)'{-h,--help}'[help for forget]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_generate {
  _arguments \
    '--bash-completion[write bash completion `file`]:' \
    '(-h --help)'{-h,--help}'[help for generate]' \
    '--man[write man pages to `directory`]:' \
    '--zsh-completion[write zsh completion `file`]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_help {
  _arguments \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_init {
  _arguments \
    '--copy-chunker-params[copy chunker parameters from the secondary repository (useful with the copy command)]' \
    '(-h --help)'{-h,--help}'[help for init]' \
    '--key-hint2[key ID of key to try decrypting the secondary repository first (default: $RESTIC_KEY_HINT2)]:' \
    '--password-command2[shell `command` to obtain the secondary repository password from (default: $RESTIC_PASSWORD_COMMAND2)]:' \
    '--password-file2[`file` to read the secondary repository password from (default: $RESTIC_PASSWORD_FILE2)]:' \
    '--repo2[secondary `repository` to copy chunker parameters from (default: $RESTIC_REPOSITORY2)]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_key {
  _arguments \
    '(-h --help)'{-h,--help}'[help for key]' \
    '--host[the hostname for new keys]:' \
    '--new-password-file[`file` from which to read the new password]:' \
    '--user[the username for new keys]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_list {
  _arguments \
    '(-h --help)'{-h,--help}'[help for list]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_ls {
  _arguments \
    '(-h --help)'{-h,--help}'[help for ls]' \
    '(*-H *--host)'{\*-H,\*--host}'[only consider snapshots for this `host`, when no snapshot ID is given (can be specified multiple times)]:' \
    '(-l --long)'{-l,--long}'[use a long listing format showing size and mode]' \
    '*--path[only consider snapshots which include this (absolute) `path`, when no snapshot ID is given]:' \
    '--recursive[include files in subfolders of the listed directories]' \
    '--tag[only consider snapshots which include this `taglist`, when no snapshot ID is given]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_migrate {
  _arguments \
    '(-f --force)'{-f,--force}'[apply a migration a second time]' \
    '(-h --help)'{-h,--help}'[help for migrate]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_mount {
  _arguments \
    '--allow-other[allow other users to access the data in the mounted directory]' \
    '(-h --help)'{-h,--help}'[help for mount]' \
    '(*-H *--host)'{\*-H,\*--host}'[only consider snapshots for this host (can be specified multiple times)]:' \
    '--no-default-permissions[for '\''allow-other'\'', ignore Unix permissions and allow users to read all snapshot files]' \
    '--owner-root[use '\''root'\'' as the owner of files and dirs]' \
    '*--path[only consider snapshots which include this (absolute) `path`]:' \
    '--snapshot-template[set `template` to use for snapshot dirs]:' \
    '--tag[only consider snapshots which include this `taglist`]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_prune {
  _arguments \
    '(-h --help)'{-h,--help}'[help for prune]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_rebuild-index {
  _arguments \
    '(-h --help)'{-h,--help}'[help for rebuild-index]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_recover {
  _arguments \
    '(-h --help)'{-h,--help}'[help for recover]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_restore {
  _arguments \
    '(*-e *--exclude)'{\*-e,\*--exclude}'[exclude a `pattern` (can be specified multiple times)]:' \
    '(-h --help)'{-h,--help}'[help for restore]' \
    '(*-H *--host)'{\*-H,\*--host}'[only consider snapshots for this host when the snapshot ID is "latest" (can be specified multiple times)]:' \
    '*--iexclude[same as `--exclude` but ignores the casing of filenames]:' \
    '*--iinclude[same as `--include` but ignores the casing of filenames]:' \
    '(*-i *--include)'{\*-i,\*--include}'[include a `pattern`, exclude everything else (can be specified multiple times)]:' \
    '*--path[only consider snapshots which include this (absolute) `path` for snapshot ID "latest"]:' \
    '--tag[only consider snapshots which include this `taglist` for snapshot ID "latest"]:' \
    '(-t --target)'{-t,--target}'[directory to extract data to]:' \
    '--verify[verify restored files content]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_self-update {
  _arguments \
    '(-h --help)'{-h,--help}'[help for self-update]' \
    '--output[Save the downloaded file as `filename` (default: running binary itself)]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_snapshots {
  _arguments \
    '(-c --compact)'{-c,--compact}'[use compact output format]' \
    '(-g --group-by)'{-g,--group-by}'[string for grouping snapshots by host,paths,tags]:' \
    '(-h --help)'{-h,--help}'[help for snapshots]' \
    '(*-H *--host)'{\*-H,\*--host}'[only consider snapshots for this `host` (can be specified multiple times)]:' \
    '--last[only show the last snapshot for each host and path]' \
    '*--path[only consider snapshots for this `path` (can be specified multiple times)]:' \
    '--tag[only consider snapshots which include this `taglist` (can be specified multiple times)]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_stats {
  _arguments \
    '(-h --help)'{-h,--help}'[help for stats]' \
    '(*-H *--host)'{\*-H,\*--host}'[only consider snapshots with the given `host` (can be specified multiple times)]:' \
    '--mode[counting mode: restore-size (default), files-by-contents, blobs-per-file or raw-data]:' \
    '*--path[only consider snapshots which include this (absolute) `path` (can be specified multiple times)]:' \
    '--tag[only consider snapshots which include this `taglist` in the format `tag[,tag,...]` (can be specified multiple times)]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_tag {
  _arguments \
    '*--add[`tag` which will be added to the existing tags (can be given multiple times)]:' \
    '(-h --help)'{-h,--help}'[help for tag]' \
    '(*-H *--host)'{\*-H,\*--host}'[only consider snapshots for this `host`, when no snapshot ID is given (can be specified multiple times)]:' \
    '*--path[only consider snapshots which include this (absolute) `path`, when no snapshot-ID is given]:' \
    '*--remove[`tag` which will be removed from the existing tags (can be given multiple times)]:' \
    '*--set[`tag` which will replace the existing tags (can be given multiple times)]:' \
    '--tag[only consider snapshots which include this `taglist`, when no snapshot-ID is given]:' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_unlock {
  _arguments \
    '(-h --help)'{-h,--help}'[help for unlock]' \
    '--remove-all[remove all locks, even non-stale ones]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

function _restic_version {
  _arguments \
    '(-h --help)'{-h,--help}'[help for version]' \
    '*--cacert[`file` to load root certificates from (default: use system certificates)]:' \
    '--cache-dir[set the cache `directory`. (default: use system default cache directory)]:' \
    '--cleanup-cache[auto remove old cache directories]' \
    '--json[set output mode to JSON for commands that support it]' \
    '--key-hint[`key` ID of key to try decrypting first (default: $RESTIC_KEY_HINT)]:' \
    '--limit-download[limits downloads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--limit-upload[limits uploads to a maximum rate in KiB/s. (default: unlimited)]:' \
    '--no-cache[do not use a local cache]' \
    '--no-lock[do not lock the repository, this allows some operations on read-only repositories]' \
    '(*-o *--option)'{\*-o,\*--option}'[set extended option (`key=value`, can be specified multiple times)]:' \
    '--password-command[shell `command` to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)]:' \
    '(-p --password-file)'{-p,--password-file}'[`file` to read the repository password from (default: $RESTIC_PASSWORD_FILE)]:' \
    '(-q --quiet)'{-q,--quiet}'[do not output comprehensive progress report]' \
    '(-r --repo)'{-r,--repo}'[`repository` to backup to or restore from (default: $RESTIC_REPOSITORY)]:' \
    '--repository-file[`file` to read the repository location from (default: $RESTIC_REPOSITORY_FILE)]:' \
    '--tls-client-cert[path to a `file` containing PEM encoded TLS client certificate and private key]:' \
    '(-v --verbose)'{-v,--verbose}'[be verbose (specify multiple times or a level using --verbose=`n`, max level/times is 3)]'
}

