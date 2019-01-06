# bash completion for restic                               -*- shell-script -*-

__restic_debug()
{
    if [[ -n ${BASH_COMP_DEBUG_FILE} ]]; then
        echo "$*" >> "${BASH_COMP_DEBUG_FILE}"
    fi
}

# Homebrew on Macs have version 1.3 of bash-completion which doesn't include
# _init_completion. This is a very minimal version of that function.
__restic_init_completion()
{
    COMPREPLY=()
    _get_comp_words_by_ref "$@" cur prev words cword
}

__restic_index_of_word()
{
    local w word=$1
    shift
    index=0
    for w in "$@"; do
        [[ $w = "$word" ]] && return
        index=$((index+1))
    done
    index=-1
}

__restic_contains_word()
{
    local w word=$1; shift
    for w in "$@"; do
        [[ $w = "$word" ]] && return
    done
    return 1
}

__restic_handle_reply()
{
    __restic_debug "${FUNCNAME[0]}"
    case $cur in
        -*)
            if [[ $(type -t compopt) = "builtin" ]]; then
                compopt -o nospace
            fi
            local allflags
            if [ ${#must_have_one_flag[@]} -ne 0 ]; then
                allflags=("${must_have_one_flag[@]}")
            else
                allflags=("${flags[*]} ${two_word_flags[*]}")
            fi
            COMPREPLY=( $(compgen -W "${allflags[*]}" -- "$cur") )
            if [[ $(type -t compopt) = "builtin" ]]; then
                [[ "${COMPREPLY[0]}" == *= ]] || compopt +o nospace
            fi

            # complete after --flag=abc
            if [[ $cur == *=* ]]; then
                if [[ $(type -t compopt) = "builtin" ]]; then
                    compopt +o nospace
                fi

                local index flag
                flag="${cur%=*}"
                __restic_index_of_word "${flag}" "${flags_with_completion[@]}"
                COMPREPLY=()
                if [[ ${index} -ge 0 ]]; then
                    PREFIX=""
                    cur="${cur#*=}"
                    ${flags_completion[${index}]}
                    if [ -n "${ZSH_VERSION}" ]; then
                        # zsh completion needs --flag= prefix
                        eval "COMPREPLY=( \"\${COMPREPLY[@]/#/${flag}=}\" )"
                    fi
                fi
            fi
            return 0;
            ;;
    esac

    # check if we are handling a flag with special work handling
    local index
    __restic_index_of_word "${prev}" "${flags_with_completion[@]}"
    if [[ ${index} -ge 0 ]]; then
        ${flags_completion[${index}]}
        return
    fi

    # we are parsing a flag and don't have a special handler, no completion
    if [[ ${cur} != "${words[cword]}" ]]; then
        return
    fi

    local completions
    completions=("${commands[@]}")
    if [[ ${#must_have_one_noun[@]} -ne 0 ]]; then
        completions=("${must_have_one_noun[@]}")
    fi
    if [[ ${#must_have_one_flag[@]} -ne 0 ]]; then
        completions+=("${must_have_one_flag[@]}")
    fi
    COMPREPLY=( $(compgen -W "${completions[*]}" -- "$cur") )

    if [[ ${#COMPREPLY[@]} -eq 0 && ${#noun_aliases[@]} -gt 0 && ${#must_have_one_noun[@]} -ne 0 ]]; then
        COMPREPLY=( $(compgen -W "${noun_aliases[*]}" -- "$cur") )
    fi

    if [[ ${#COMPREPLY[@]} -eq 0 ]]; then
        declare -F __custom_func >/dev/null && __custom_func
    fi

    # available in bash-completion >= 2, not always present on macOS
    if declare -F __ltrim_colon_completions >/dev/null; then
        __ltrim_colon_completions "$cur"
    fi

    # If there is only 1 completion and it is a flag with an = it will be completed
    # but we don't want a space after the =
    if [[ "${#COMPREPLY[@]}" -eq "1" ]] && [[ $(type -t compopt) = "builtin" ]] && [[ "${COMPREPLY[0]}" == --*= ]]; then
       compopt -o nospace
    fi
}

# The arguments should be in the form "ext1|ext2|extn"
__restic_handle_filename_extension_flag()
{
    local ext="$1"
    _filedir "@(${ext})"
}

__restic_handle_subdirs_in_dir_flag()
{
    local dir="$1"
    pushd "${dir}" >/dev/null 2>&1 && _filedir -d && popd >/dev/null 2>&1
}

__restic_handle_flag()
{
    __restic_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"

    # if a command required a flag, and we found it, unset must_have_one_flag()
    local flagname=${words[c]}
    local flagvalue
    # if the word contained an =
    if [[ ${words[c]} == *"="* ]]; then
        flagvalue=${flagname#*=} # take in as flagvalue after the =
        flagname=${flagname%=*} # strip everything after the =
        flagname="${flagname}=" # but put the = back
    fi
    __restic_debug "${FUNCNAME[0]}: looking for ${flagname}"
    if __restic_contains_word "${flagname}" "${must_have_one_flag[@]}"; then
        must_have_one_flag=()
    fi

    # if you set a flag which only applies to this command, don't show subcommands
    if __restic_contains_word "${flagname}" "${local_nonpersistent_flags[@]}"; then
      commands=()
    fi

    # keep flag value with flagname as flaghash
    # flaghash variable is an associative array which is only supported in bash > 3.
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        if [ -n "${flagvalue}" ] ; then
            flaghash[${flagname}]=${flagvalue}
        elif [ -n "${words[ $((c+1)) ]}" ] ; then
            flaghash[${flagname}]=${words[ $((c+1)) ]}
        else
            flaghash[${flagname}]="true" # pad "true" for bool flag
        fi
    fi

    # skip the argument to a two word flag
    if __restic_contains_word "${words[c]}" "${two_word_flags[@]}"; then
        c=$((c+1))
        # if we are looking for a flags value, don't show commands
        if [[ $c -eq $cword ]]; then
            commands=()
        fi
    fi

    c=$((c+1))

}

__restic_handle_noun()
{
    __restic_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"

    if __restic_contains_word "${words[c]}" "${must_have_one_noun[@]}"; then
        must_have_one_noun=()
    elif __restic_contains_word "${words[c]}" "${noun_aliases[@]}"; then
        must_have_one_noun=()
    fi

    nouns+=("${words[c]}")
    c=$((c+1))
}

__restic_handle_command()
{
    __restic_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"

    local next_command
    if [[ -n ${last_command} ]]; then
        next_command="_${last_command}_${words[c]//:/__}"
    else
        if [[ $c -eq 0 ]]; then
            next_command="_restic_root_command"
        else
            next_command="_${words[c]//:/__}"
        fi
    fi
    c=$((c+1))
    __restic_debug "${FUNCNAME[0]}: looking for ${next_command}"
    declare -F "$next_command" >/dev/null && $next_command
}

__restic_handle_word()
{
    if [[ $c -ge $cword ]]; then
        __restic_handle_reply
        return
    fi
    __restic_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"
    if [[ "${words[c]}" == -* ]]; then
        __restic_handle_flag
    elif __restic_contains_word "${words[c]}" "${commands[@]}"; then
        __restic_handle_command
    elif [[ $c -eq 0 ]]; then
        __restic_handle_command
    elif __restic_contains_word "${words[c]}" "${command_aliases[@]}"; then
        # aliashash variable is an associative array which is only supported in bash > 3.
        if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
            words[c]=${aliashash[${words[c]}]}
            __restic_handle_command
        else
            __restic_handle_noun
        fi
    else
        __restic_handle_noun
    fi
    __restic_handle_word
}

_restic_backup()
{
    last_command="restic_backup"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--exclude=")
    two_word_flags+=("-e")
    local_nonpersistent_flags+=("--exclude=")
    flags+=("--exclude-caches")
    local_nonpersistent_flags+=("--exclude-caches")
    flags+=("--exclude-file=")
    local_nonpersistent_flags+=("--exclude-file=")
    flags+=("--exclude-if-present=")
    local_nonpersistent_flags+=("--exclude-if-present=")
    flags+=("--files-from=")
    local_nonpersistent_flags+=("--files-from=")
    flags+=("--force")
    flags+=("-f")
    local_nonpersistent_flags+=("--force")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--host=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--host=")
    flags+=("--one-file-system")
    flags+=("-x")
    local_nonpersistent_flags+=("--one-file-system")
    flags+=("--parent=")
    local_nonpersistent_flags+=("--parent=")
    flags+=("--stdin")
    local_nonpersistent_flags+=("--stdin")
    flags+=("--stdin-filename=")
    local_nonpersistent_flags+=("--stdin-filename=")
    flags+=("--tag=")
    local_nonpersistent_flags+=("--tag=")
    flags+=("--time=")
    local_nonpersistent_flags+=("--time=")
    flags+=("--with-atime")
    local_nonpersistent_flags+=("--with-atime")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_cache()
{
    last_command="restic_cache"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--cleanup")
    local_nonpersistent_flags+=("--cleanup")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--max-age=")
    local_nonpersistent_flags+=("--max-age=")
    flags+=("--no-size")
    local_nonpersistent_flags+=("--no-size")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_cat()
{
    last_command="restic_cat"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_check()
{
    last_command="restic_check"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--check-unused")
    local_nonpersistent_flags+=("--check-unused")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--read-data")
    local_nonpersistent_flags+=("--read-data")
    flags+=("--read-data-subset=")
    local_nonpersistent_flags+=("--read-data-subset=")
    flags+=("--with-cache")
    local_nonpersistent_flags+=("--with-cache")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_diff()
{
    last_command="restic_diff"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--metadata")
    local_nonpersistent_flags+=("--metadata")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_dump()
{
    last_command="restic_dump"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--host=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--host=")
    flags+=("--path=")
    local_nonpersistent_flags+=("--path=")
    flags+=("--tag=")
    local_nonpersistent_flags+=("--tag=")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_find()
{
    last_command="restic_find"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--blob")
    local_nonpersistent_flags+=("--blob")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--host=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--host=")
    flags+=("--ignore-case")
    flags+=("-i")
    local_nonpersistent_flags+=("--ignore-case")
    flags+=("--long")
    flags+=("-l")
    local_nonpersistent_flags+=("--long")
    flags+=("--newest=")
    two_word_flags+=("-N")
    local_nonpersistent_flags+=("--newest=")
    flags+=("--oldest=")
    two_word_flags+=("-O")
    local_nonpersistent_flags+=("--oldest=")
    flags+=("--pack")
    local_nonpersistent_flags+=("--pack")
    flags+=("--path=")
    local_nonpersistent_flags+=("--path=")
    flags+=("--show-pack-id")
    local_nonpersistent_flags+=("--show-pack-id")
    flags+=("--snapshot=")
    two_word_flags+=("-s")
    local_nonpersistent_flags+=("--snapshot=")
    flags+=("--tag=")
    local_nonpersistent_flags+=("--tag=")
    flags+=("--tree")
    local_nonpersistent_flags+=("--tree")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_forget()
{
    last_command="restic_forget"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--keep-last=")
    two_word_flags+=("-l")
    local_nonpersistent_flags+=("--keep-last=")
    flags+=("--keep-hourly=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--keep-hourly=")
    flags+=("--keep-daily=")
    two_word_flags+=("-d")
    local_nonpersistent_flags+=("--keep-daily=")
    flags+=("--keep-weekly=")
    two_word_flags+=("-w")
    local_nonpersistent_flags+=("--keep-weekly=")
    flags+=("--keep-monthly=")
    two_word_flags+=("-m")
    local_nonpersistent_flags+=("--keep-monthly=")
    flags+=("--keep-yearly=")
    two_word_flags+=("-y")
    local_nonpersistent_flags+=("--keep-yearly=")
    flags+=("--keep-within=")
    local_nonpersistent_flags+=("--keep-within=")
    flags+=("--keep-tag=")
    local_nonpersistent_flags+=("--keep-tag=")
    flags+=("--host=")
    local_nonpersistent_flags+=("--host=")
    flags+=("--tag=")
    local_nonpersistent_flags+=("--tag=")
    flags+=("--path=")
    local_nonpersistent_flags+=("--path=")
    flags+=("--compact")
    flags+=("-c")
    local_nonpersistent_flags+=("--compact")
    flags+=("--group-by=")
    two_word_flags+=("-g")
    local_nonpersistent_flags+=("--group-by=")
    flags+=("--dry-run")
    flags+=("-n")
    local_nonpersistent_flags+=("--dry-run")
    flags+=("--prune")
    local_nonpersistent_flags+=("--prune")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_generate()
{
    last_command="restic_generate"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--bash-completion=")
    local_nonpersistent_flags+=("--bash-completion=")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--man=")
    local_nonpersistent_flags+=("--man=")
    flags+=("--zsh-completion=")
    local_nonpersistent_flags+=("--zsh-completion=")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_init()
{
    last_command="restic_init"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_key()
{
    last_command="restic_key"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--new-password-file=")
    local_nonpersistent_flags+=("--new-password-file=")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_list()
{
    last_command="restic_list"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_ls()
{
    last_command="restic_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--host=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--host=")
    flags+=("--long")
    flags+=("-l")
    local_nonpersistent_flags+=("--long")
    flags+=("--path=")
    local_nonpersistent_flags+=("--path=")
    flags+=("--recursive")
    local_nonpersistent_flags+=("--recursive")
    flags+=("--tag=")
    local_nonpersistent_flags+=("--tag=")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_migrate()
{
    last_command="restic_migrate"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--force")
    flags+=("-f")
    local_nonpersistent_flags+=("--force")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_mount()
{
    last_command="restic_mount"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--allow-other")
    local_nonpersistent_flags+=("--allow-other")
    flags+=("--allow-root")
    local_nonpersistent_flags+=("--allow-root")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--host=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--host=")
    flags+=("--no-default-permissions")
    local_nonpersistent_flags+=("--no-default-permissions")
    flags+=("--owner-root")
    local_nonpersistent_flags+=("--owner-root")
    flags+=("--path=")
    local_nonpersistent_flags+=("--path=")
    flags+=("--snapshot-template=")
    local_nonpersistent_flags+=("--snapshot-template=")
    flags+=("--tag=")
    local_nonpersistent_flags+=("--tag=")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_prune()
{
    last_command="restic_prune"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_rebuild-index()
{
    last_command="restic_rebuild-index"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_recover()
{
    last_command="restic_recover"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_restore()
{
    last_command="restic_restore"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--exclude=")
    two_word_flags+=("-e")
    local_nonpersistent_flags+=("--exclude=")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--host=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--host=")
    flags+=("--include=")
    two_word_flags+=("-i")
    local_nonpersistent_flags+=("--include=")
    flags+=("--path=")
    local_nonpersistent_flags+=("--path=")
    flags+=("--tag=")
    local_nonpersistent_flags+=("--tag=")
    flags+=("--target=")
    two_word_flags+=("-t")
    local_nonpersistent_flags+=("--target=")
    flags+=("--verify")
    local_nonpersistent_flags+=("--verify")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_self-update()
{
    last_command="restic_self-update"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--output=")
    local_nonpersistent_flags+=("--output=")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_snapshots()
{
    last_command="restic_snapshots"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--compact")
    flags+=("-c")
    local_nonpersistent_flags+=("--compact")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--host=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--host=")
    flags+=("--last")
    local_nonpersistent_flags+=("--last")
    flags+=("--path=")
    local_nonpersistent_flags+=("--path=")
    flags+=("--tag=")
    local_nonpersistent_flags+=("--tag=")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_stats()
{
    last_command="restic_stats"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--host=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--host=")
    flags+=("--mode=")
    local_nonpersistent_flags+=("--mode=")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_tag()
{
    last_command="restic_tag"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--add=")
    local_nonpersistent_flags+=("--add=")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--host=")
    two_word_flags+=("-H")
    local_nonpersistent_flags+=("--host=")
    flags+=("--path=")
    local_nonpersistent_flags+=("--path=")
    flags+=("--remove=")
    local_nonpersistent_flags+=("--remove=")
    flags+=("--set=")
    local_nonpersistent_flags+=("--set=")
    flags+=("--tag=")
    local_nonpersistent_flags+=("--tag=")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_unlock()
{
    last_command="restic_unlock"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--remove-all")
    local_nonpersistent_flags+=("--remove-all")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_version()
{
    last_command="restic_version"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_restic_root_command()
{
    last_command="restic"

    command_aliases=()

    commands=()
    commands+=("backup")
    commands+=("cache")
    commands+=("cat")
    commands+=("check")
    commands+=("diff")
    commands+=("dump")
    commands+=("find")
    commands+=("forget")
    commands+=("generate")
    commands+=("init")
    commands+=("key")
    commands+=("list")
    commands+=("ls")
    commands+=("migrate")
    commands+=("mount")
    commands+=("prune")
    commands+=("rebuild-index")
    commands+=("recover")
    commands+=("restore")
    commands+=("self-update")
    commands+=("snapshots")
    commands+=("stats")
    commands+=("tag")
    commands+=("unlock")
    commands+=("version")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--cacert=")
    flags+=("--cache-dir=")
    flags+=("--cleanup-cache")
    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    flags+=("--json")
    flags+=("--key-hint=")
    flags+=("--limit-download=")
    flags+=("--limit-upload=")
    flags+=("--no-cache")
    flags+=("--no-lock")
    flags+=("--option=")
    two_word_flags+=("-o")
    flags+=("--password-command=")
    flags+=("--password-file=")
    two_word_flags+=("-p")
    flags+=("--quiet")
    flags+=("-q")
    flags+=("--repo=")
    two_word_flags+=("-r")
    flags+=("--tls-client-cert=")
    flags+=("--verbose")
    flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

__start_restic()
{
    local cur prev words cword
    declare -A flaghash 2>/dev/null || :
    declare -A aliashash 2>/dev/null || :
    if declare -F _init_completion >/dev/null 2>&1; then
        _init_completion -s || return
    else
        __restic_init_completion -n "=" || return
    fi

    local c=0
    local flags=()
    local two_word_flags=()
    local local_nonpersistent_flags=()
    local flags_with_completion=()
    local flags_completion=()
    local commands=("restic")
    local must_have_one_flag=()
    local must_have_one_noun=()
    local last_command
    local nouns=()

    __restic_handle_word
}

if [[ $(type -t compopt) = "builtin" ]]; then
    complete -o default -F __start_restic restic
else
    complete -o default -o nospace -F __start_restic restic
fi

# ex: ts=4 sw=4 et filetype=sh
