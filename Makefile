.PHONY: clean all test

test:
	go test
	$(MAKE) -C cmd/khepri
	test/run.sh cmd/khepri/khepri

clean:
	go clean
	$(MAKE) -C cmd/khepri test
