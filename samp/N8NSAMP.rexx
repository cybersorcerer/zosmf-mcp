/* REXX */
call outtrap out.
address tso 'listc'
do i=1 to out.0
	say out.i
end

exit 0