// Command sundog-dialog prints expanded dialog for review.
//
// It exists because the corpus is composed rather than stored as sentences: a
// translator editing docs/i18n/atari-st-dialog-zh-Hant.json edits fragments,
// and the only way to judge the result is to read the sentences those
// fragments actually build. Printing both the first and the last alternative
// shows the range the original varies across.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

func main() {
	source := flag.Bool("source", false, "印原始英文而非繁中譯文")
	block := flag.Int("block", 0, "只印這個區塊編號（例如 0x10）；0 表示全部")
	flag.Parse()

	data, err := load(*source)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	last := func(n int) int { return n - 1 }
	for _, entry := range data.Blocks() {
		if *block != 0 && entry.Number != *block {
			continue
		}
		fmt.Printf("== 區塊 0x%02x（raw 0x%05x，%d 句）\n", entry.Number, entry.RawOffset, len(entry.Lines))
		for _, line := range entry.Lines {
			first, err := data.Expand(entry, line.Tokens, atarist.FirstAlternative, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "區塊 0x%02x 第 %d 句：%v\n", entry.Number, line.Index, err)
				os.Exit(1)
			}
			alternate, err := data.Expand(entry, line.Tokens, last, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "區塊 0x%02x 第 %d 句：%v\n", entry.Number, line.Index, err)
				os.Exit(1)
			}
			fmt.Printf("  %2d  %s\n", line.Index, first)
			if alternate != first {
				fmt.Printf("      %s\n", alternate)
			}
		}
		fmt.Println()
	}
}

func load(source bool) (atarist.DialogData, error) {
	if source {
		return atarist.EmbeddedDialogData()
	}
	return atarist.EmbeddedTranslatedDialogData()
}
