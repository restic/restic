prepare
run k backup "${BASE}/fake-data"
run k restore "$(k list ref)" "${BASE}/fake-data-restore"
diff -aur "${BASE}/fake-data" "${BASE}/fake-data-restore"
cleanup
