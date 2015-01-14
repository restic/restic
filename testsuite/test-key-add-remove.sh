set -e

dump_repo() {
    if [ "$FAILED" == "1" ]; then
        tar cvz "$RESTIC_REPOSITORY" | base64 >&2
    fi
}

FAILED=1

trap dump_repo 0

prepare
unset RESTIC_PASSWORD
RESTIC_PASSWORD=foo run restic init
RESTIC_PASSWORD=foo run restic key list

RESTIC_PASSWORD=foo RESTIC_NEWPASSWORD=foobar run restic key change
RESTIC_PASSWORD=foobar run restic key list
RESTIC_PASSWORD=foobar RESTIC_NEWPASSWORD=foo run restic key change

OLD_PWD=foo
for i in {1..3}; do
    NEW_PWD=bar$i
    RESTIC_PASSWORD=$OLD_PWD RESTIC_NEWPASSWORD=$NEW_PWD run restic key add
    RESTIC_PASSWORD=$OLD_PWD run restic key list
    RESTIC_PASSWORD=$NEW_PWD run restic key list

    export RESTIC_PASSWORD=$OLD_PWD
    ID=$(restic key list | grep '^\*'|cut -d ' ' -f 1| sed 's/^.//')
    unset RESTIC_PASSWORD
    RESTIC_PASSWORD=$NEW_PWD run restic key rm $ID
    RESTIC_PASSWORD=$NEW_PWD run restic key list

    OLD_PWD=bar$i
done

RESTIC_PASSWORD=$OLD_PWD run restic fsck -o --check-data

cleanup

FAILED=0
