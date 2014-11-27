set -e

dump_repo() {
    if [ "$FAILED" == "1" ]; then
        tar cvz "$KHEPRI_REPOSITORY" | base64 >&2
    fi
}

FAILED=1

trap dump_repo 0

prepare
unset KHEPRI_PASSWORD
KHEPRI_PASSWORD=foo run khepri init
KHEPRI_PASSWORD=foo run khepri key list
OLD_PWD=foo
for i in {1..3}; do
    NEW_PWD=bar$i
    KHEPRI_PASSWORD=$OLD_PWD KHEPRI_NEWPASSWORD=$NEW_PWD run khepri key add
    KHEPRI_PASSWORD=$OLD_PWD run khepri key list
    KHEPRI_PASSWORD=$NEW_PWD run khepri key list

    export KHEPRI_PASSWORD=$OLD_PWD
    ID=$(khepri key list | grep '^\*'|cut -d ' ' -f 1| sed 's/^.//')
    unset KHEPRI_PASSWORD
    KHEPRI_PASSWORD=$NEW_PWD run khepri key rm $ID
    KHEPRI_PASSWORD=$NEW_PWD run khepri key list

    OLD_PWD=bar$i
done

cleanup

FAILED=0
