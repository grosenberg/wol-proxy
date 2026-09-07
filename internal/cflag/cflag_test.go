package cflag

import (
	"reflect"
	"testing"
)

func TestSortFlags(t *testing.T) {
	input := []string{"v", "save", "R", "S", "r", "s", "Save", "reload", "Restart", "c"}
	expected := []string{"c", "r", "reload", "R", "Restart", "s", "save", "S", "Save", "v"}

	result := sortFlags(input)

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("sortFlags(%v)\ngot:  %v\nwant: %v", input, result, expected)
	}
}
