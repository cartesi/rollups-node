ADDRESS_BOOK="$(sh tools/format_addresses.sh)"
awk -i inplace -vADDRESS_BOOK="${ADDRESS_BOOK}" '
	/@ADDRESS-BOOK-BEGIN@/ {
		print; # restore delimiter
		print(ADDRESS_BOOK);
		p=1;
		next;
	}
	/@ADDRESS-BOOK-END@/ {
		print; # restore delimiter
		p=0;
		next;
	}
	!p
' Makefile
