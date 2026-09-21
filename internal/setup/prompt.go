package setup

import (
	"errors"
	"io"
)

// AskAll walks the catalog and asks for each field. Task 6 implements it.
func AskAll(fields []Field, v *Values, in io.Reader, out io.Writer) error {
	return errors.New("the interactive wizard is not implemented yet — pass the fields as flags")
}
