// Copyright 2014 Alexander Neumann. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package chunker implements Content Defined Chunking (CDC) based on a rolling
Rabin Checksum.

Choosing a Random Irreducible Polynomial

The function RandomPolynomial() returns a new random polynomial of degree 53
for use with the chunker. The degree 53 is chosen because it is the largest
prime below 64-8 = 56, so that the top 8 bits of an uint64 can be used for
optimising calculations in the chunker.

A random polynomial is chosen selecting 64 random bits, masking away bits
64..54 and setting bit 53 to one (otherwise the polynomial is not of the
desired degree) and bit 0 to one (otherwise the polynomial is trivially
reducible), so that 51 bits are chosen at random.

This process is repeated until Irreducible() returns true, then this
polynomials is returned. If this doesn't happen after 1 million tries, the
function returns an error. The probability for selecting an irreducible
polynomial at random is about 7.5% ( (2^53-2)/53 / 2^51), so the probability
that no irreducible polynomial has been found after 100 tries is lower than
0.04%.

Verifying Irreducible Polynomials

During development the results have been verified using the computational
discrete algebra system GAP, which can be obtained from the website at
http://www.gap-system.org/.

For filtering a given list of polynomials in hexadecimal coefficient notation,
the following script can be used:

	# create x over F_2 = GF(2)
	x := Indeterminate(GF(2), "x");

	# test if polynomial is irreducible, i.e. the number of factors is one
	IrredPoly := function (poly)
		return (Length(Factors(poly)) = 1);
	end;;

	# create a polynomial in x from the hexadecimal representation of the
	# coefficients
	Hex2Poly := function (s)
		return ValuePol(CoefficientsQadic(IntHexString(s), 2), x);
	end;;

	# list of candidates, in hex
	candidates := [ "3DA3358B4DC173" ];

	# create real polynomials
	L := List(candidates, Hex2Poly);

	# filter and display the list of irreducible polynomials contained in L
	Display(Filtered(L, x -> (IrredPoly(x))));

All irreducible polynomials from the list are written to the output.

Background Literature

An introduction to Rabin Fingerprints/Checksums can be found in the following articles:

Michael O. Rabin (1981): "Fingerprinting by Random Polynomials"
http://www.xmailserver.org/rabin.pdf

Ross N. Williams (1993): "A Painless Guide to CRC Error Detection Algorithms"
http://www.zlib.net/crc_v3.txt

Andrei Z. Broder (1993): "Some Applications of Rabin's Fingerprinting Method"
http://www.xmailserver.org/rabin_apps.pdf

Shuhong Gao and Daniel Panario (1997): "Tests and Constructions of Irreducible Polynomials over Finite Fields"
http://www.math.clemson.edu/~sgao/papers/GP97a.pdf

Andrew Kadatch, Bob Jenkins (2007): "Everything we know about CRC but afraid to forget"
http://crcutil.googlecode.com/files/crc-doc.1.0.pdf

*/
package chunker
