package cflag

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
)

// embed to wrap and replace the default PrintDefaults() and Usage() methods
type CliFlagSet struct {
	*flag.FlagSet
}

// CmdLine is the default set of command-line flags, parsed from [os.Args].
// The top-level functions such as [BoolVar], [Arg], and so on are wrappers for the
// methods of CommandLine.
var CmdLine *CliFlagSet

func NewCFlagSet(name string) *CliFlagSet {
	CmdLine = &CliFlagSet{
		FlagSet: flag.NewFlagSet(name, flag.ExitOnError),
	}
	CmdLine.Usage = Usage
	return CmdLine
}

// Usage prints a usage message documenting all defined command-line flags
// to [CommandLine]'s output, which by default is [os.Stderr].
// It is called when an error occurs while parsing flags.
// The function is a variable that may be changed to point to a custom function.
// By default it prints a simple header and calls [PrintDefaults]; for details about the
// format of the output and how to control it, see the documentation for [PrintDefaults].
// Custom usage functions may choose to exit the program; by default exiting
// happens anyway as the command line's error handling strategy is set to
// [ExitOnError].
var Usage = func() {
	fmt.Fprintf(CmdLine.Output(), "Usage of %s:\n", os.Args[0])
	CmdLine.PrintDefaults()
}

// Parse parses the command-line flags from [os.Args][1:]. Must be called
// after all flags are defined and before flags are accessed by the program.
func Parse() {
	// Ignore errors; CommandLine is set for ExitOnError.
	CmdLine.Parse(os.Args[1:])
}

// copied from flag.PrintDefaults() and adjusted for custom sorting
func (cfs *CliFlagSet) PrintDefaults() {
	var isZeroValueErrs []error

	// collect all flag names into a slice
	var flagNames []string
	flagMap := make(map[string]*flag.Flag)

	cfs.VisitAll(func(f *flag.Flag) {
		flagNames = append(flagNames, f.Name)
		flagMap[f.Name] = f
	})

	flagNames = sortFlags(flagNames)

	for idx := range len(flagNames) {
		flgName := flagNames[idx]
		flg := flagMap[flgName]

		var b strings.Builder
		fmt.Fprintf(&b, "  -%s", flg.Name) // Two spaces before -; see next two comments.
		name, usage := flag.UnquoteUsage(flg)
		if len(name) > 0 {
			b.WriteString(" ")
			b.WriteString(name)
		}
		// Boolean flags of one ASCII letter are so common we
		// treat them specially, putting their usage on the same line.
		if b.Len() <= 4 { // space, space, '-', 'x'.
			b.WriteString("\t")
		} else {
			// Four spaces before the tab triggers good alignment
			// for both 4- and 8-space tab stops.
			b.WriteString("\n    \t")
		}
		b.WriteString(strings.ReplaceAll(usage, "\n", "\n    \t"))

		// Print the default value only if it differs to the zero value
		// for this flag type.
		if isZero, err := isZeroValue(flg, flg.DefValue); err != nil {
			isZeroValueErrs = append(isZeroValueErrs, err)
		} else if !isZero {
			var isString bool
			if getter, ok := flg.Value.(flag.Getter); ok {
				_, isString = getter.Get().(string)
			}
			if isString {
				// put quotes on the value
				fmt.Fprintf(&b, " (default %q)", flg.DefValue)
			} else {
				fmt.Fprintf(&b, " (default %v)", flg.DefValue)
			}
		}
		fmt.Fprint(cfs.Output(), b.String(), "\n")
	}

	// If calling String on any zero flag.Values triggered a panic, print
	// the messages after the full set of defaults so that the programmer
	// knows to fix the panic.
	if errs := isZeroValueErrs; len(errs) > 0 {
		fmt.Fprintln(cfs.Output())
		for _, err := range errs {
			fmt.Fprintln(cfs.Output(), err)
		}
	}
}

// sortFlags sorts flag names alphabetically by first letter with the ordering
// 'a' < 'A' < 'b' < 'B' ... < 'r' < 'R' < 's' < 'S' < 't' < 'T' ...,
// grouping all words by first letter and breaking ties alphabetically.
func sortFlags(flagNames []string) []string {
	sort.Slice(flagNames, func(i, j int) bool {
		r1 := firstRune(flagNames[i])
		r2 := firstRune(flagNames[j])
		rank1 := letterRank(r1)
		rank2 := letterRank(r2)
		if rank1 != rank2 {
			return rank1 < rank2
		}
		return flagNames[i] < flagNames[j]
	})
	return flagNames
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

func letterRank(r rune) int {
	switch {
	case r >= 'a' && r <= 'z':
		return 1000 + int(r-'a')*2
	case r >= 'A' && r <= 'Z':
		return 1000 + int(r-'A')*2 + 1
	default:
		return int(r)
	}
}

// copied from flag.isZeroValue()
// isZeroValue determines whether the string represents the zero
// value for a flag.
func isZeroValue(flg *flag.Flag, value string) (ok bool, err error) {
	// Build a zero value of the flag's Value type, and see if the
	// result of calling its String method equals the value passed in.
	// This works unless the Value type is itself an interface type.
	typ := reflect.TypeOf(flg.Value)
	var z reflect.Value
	if typ.Kind() == reflect.Pointer {
		z = reflect.New(typ.Elem())
	} else {
		z = reflect.Zero(typ)
	}
	// Catch panics calling the String method, which shouldn't prevent the
	// usage message from being printed, but that we should report to the
	// user so that they know to fix their code.
	defer func() {
		if e := recover(); e != nil {
			if typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			err = fmt.Errorf("panic calling String method on zero %v for flag %s: %v", typ, flg.Name, e)
		}
	}()
	return value == z.Interface().(flag.Value).String(), nil
}

// func init() {
// 	// It's possible for execl to hand us an empty os.Args.
// 	if len(os.Args) == 0 {
// 		CliFlagSet = NewCFlagSet("")
// 	} else {
// 		CliFlagSet = NewCFlagSet(os.Args[0])
// 	}

// 	// Override generic FlagSet default Usage with call to global Usage.
// 	// Note: This is not CommandLine.Usage = Usage,
// 	// because we want any eventual call to use any updated value of Usage,
// 	// not the value it has when this line is run.
// 	CliFlagSet.Usage = CliFlagsUsage
// }
