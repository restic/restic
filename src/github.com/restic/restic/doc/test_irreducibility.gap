# This file is a script for GAP and tests a list of polynomials in hexadecimal
# for irreducibility over F_2

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
