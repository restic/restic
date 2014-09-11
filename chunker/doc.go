// Copyright 2014 Alexander Neumann. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package chunker implements Content Defined Chunking (CDC) based on a rolling
Rabin Checksum.

Background Literature

An introduction to Rabin Fingerprints/Checksums can be found in the following articles:

Michael O. Rabin (1981): "Fingerprinting by Random Polynomials"
http://www.xmailserver.org/rabin.pdf

Ross N. Williams (1993): "A Painless Guide to CRC Error Detection Algorithms"
http://www.zlib.net/crc_v3.txt

Andrei Z. Broder (1993): "Some Applications of Rabin's Fingerprinting Method"
http://www.xmailserver.org/rabin_apps.pdf

Andrew Kadatch, Bob Jenkins (2007): "Everything we know about CRC but afraid to forget"
http://crcutil.googlecode.com/files/crc-doc.1.0.pdf

*/
package chunker
