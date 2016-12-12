Backblaze B2 backend support for restic
=======================================

This package allows using Backblaze's B2 object storage service as a backend for
restic. It implements kurin's [blazer](https://github.com/kurin/blazer) client
library for integration with the B2 service.

This package includes unit tests, which are included with other project tests
and run with:

```
gb test
```

To test B2 backend support, you can run all restic integration tests against the
actual B2 API. To do so, provide a B2 account ID and secret key when running
tests.

For example, the following shell command provides the B2 credentials in the
`B2_ACCOUNT_ID` and `B2_ACCOUNT_KEY` environment variables, and then runs
project tests:

```
B2_ACCOUNT_ID=1a2b3c4d5e6f B2_ACCOUNT_KEY=0123456789abcdef0123456789 gb test
```

It should be possible to run the full test suite at least once per day with a
free B2 account. Running multiple times in one day may require raising the data
caps, especially the download bandwidth cap.
