package logwriter_test

import (
	"testing"

	"github.com/draganm/wazzup/logwriter"
	"github.com/stretchr/testify/require"
)

func TestLogWriter(t *testing.T) {
	type output struct {
		lines []string
		n     int
		err   error
	}

	scenarios := []struct {
		Name         string
		Inputs       []string
		ExpectOutput output
	}{
		{
			Name:   "single line",
			Inputs: []string{"abc\n"},
			ExpectOutput: output{
				lines: []string{"abc"},
				n:     4,
			},
		},
		{
			Name:   "single line - no EOL",
			Inputs: []string{"abc"},
			ExpectOutput: output{
				lines: []string{"abc"},
				n:     3,
			},
		},
		{
			Name:   "single line - two parts",
			Inputs: []string{"ab", "c\n"},
			ExpectOutput: output{
				lines: []string{"abc"},
				n:     4,
			},
		},
		{
			Name:   "single line - three parts",
			Inputs: []string{"a", "b", "c\n"},
			ExpectOutput: output{
				lines: []string{"abc"},
				n:     4,
			},
		},
		{
			Name:   "single line - three parts, EOL separately",
			Inputs: []string{"a", "b", "c", "\n"},
			ExpectOutput: output{
				lines: []string{"abc"},
				n:     4,
			},
		},
		{
			Name:   "two lines at once",
			Inputs: []string{"abc\ndef\n"},
			ExpectOutput: output{
				lines: []string{"abc", "def"},
				n:     8,
			},
		},
		{
			Name:   "two lines at once, not end EOL",
			Inputs: []string{"abc\ndef"},
			ExpectOutput: output{
				lines: []string{"abc", "def"},
				n:     7,
			},
		},
	}

	for _, sc := range scenarios {

		t.Run(sc.Name, func(t *testing.T) {
			lines := []string(nil)

			var err error
			var n int

			lw := logwriter.New(func(ls []string) error {
				lines = append(lines, ls...)
				return nil
			})

			for _, i := range sc.Inputs {
				var rn int
				rn, err = lw.Write([]byte(i))
				n += rn

				if err != nil {
					break
				}
			}

			if err == nil {
				err = lw.Close()
			}

			require.Equal(t, sc.ExpectOutput, output{
				lines: lines,
				n:     n,
				err:   err,
			})

		})
	}
}
