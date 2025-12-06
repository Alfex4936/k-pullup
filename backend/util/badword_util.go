package util

import (
	"bufio"
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

var urlRegex = regexp.MustCompile(`https?://[^\s]+`)

type BadWordUtil struct {
	BadWordsListByte [][]byte
	BadWordsList     []string
	BadWordRegex     *regexp.Regexp
	Matcher          *Matcher
	ByteMatcher      *Matcher
}

func NewBadWordUtil() *BadWordUtil {
	return &BadWordUtil{}
}

func RegisterBadWordUtilLifecycle(lifecycle fx.Lifecycle, badWordUtil *BadWordUtil, logger *zap.Logger) {
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			filePath := os.Getenv("BAD_WORDS_FILE_PATH")
			if filePath == "" {
				filePath = "badwords.txt" // Provide a default path if not set
			}
			logger.Info("Loading bad words from file", zap.String("path", filePath))
			if err := badWordUtil.LoadBadWords(filePath); err != nil {
				logger.Error("Failed to load bad words", zap.Error(err))
				return err
			}
			badWordUtil.LoadBadWordsByte(filePath)

			logger.Info("Bad words loaded successfully")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// Cleanup if necessary
			return nil
		},
	})
}

func (b *BadWordUtil) CompileBadWordsPattern() error {
	var pattern strings.Builder
	pattern.WriteString(`(`)
	for i, word := range b.BadWordsList {
		if word == "" {
			continue
		}
		pattern.WriteString(regexp.QuoteMeta(word))
		if i < len(b.BadWordsList)-1 {
			pattern.WriteString(`|`)
		}
	}
	pattern.WriteString(`)`)

	var err error
	b.BadWordRegex, err = regexp.Compile(pattern.String())
	return err
}

func (b *BadWordUtil) CheckForBadWords(input string) (bool, error) {
	if b.BadWordRegex == nil {
		b.CompileBadWordsPattern()
		return false, errors.New("bad words pattern not compiled")
	}

	return b.BadWordRegex.MatchString(input), nil
}

// USE CheckForBadWords
func (b *BadWordUtil) CheckForBadWordsWithGo(input string) (bool, error) {
	for _, word := range b.BadWordsList {
		if word == "" {
			continue
		}

		// Check if the bad word is a substring of the input
		if strings.Contains(input, word) {
			return true, nil
		}
	}
	return false, nil
}

// USE CheckForBadWords
func (b *BadWordUtil) CheckForBadWordsWithGoRoutine(input string) (bool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensures context is canceled once we return.

	resultChan := make(chan bool)
	var wg sync.WaitGroup

	for _, word := range b.BadWordsList {
		wg.Add(1)
		go func(w string) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return // Early exit on context cancellation.
			default:
				if w == "" {
					return // Skip empty words.
				}
				if strings.Contains(input, w) {
					resultChan <- true
					cancel() // Found a bad word, signal to cancel other goroutines.
				}
			}
		}(word)
	}

	// Close the resultChan once all goroutines have finished.
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results.
	for result := range resultChan {
		if result {
			return true, nil
		}
	}

	return false, nil
}

func (b *BadWordUtil) ReplaceBadWords(input string) (string, error) {
	if b.Matcher == nil {
		b.Matcher = CompileStrings(b.BadWordsList)
		return input, errors.New("bad words matcher not initialized")
	}

	mask := make([]bool, len(input))
	matchesFound := false

	b.Matcher.IterateString(input, func(word string, start int) bool {
		matchesFound = true
		end := start + len(word)
		for k := start; k < end; k++ {
			mask[k] = true
		}
		return true
	})

	if !matchesFound {
		return input, nil
	}

	var sb strings.Builder
	sb.Grow(len(input))

	i := 0
	for i < len(input) {
		r, size := utf8.DecodeRuneInString(input[i:])
		if mask[i] {
			sb.WriteRune('*')
			i += size
		} else {
			sb.WriteRune(r)
			i += size
		}
	}

	return sb.String(), nil
}

func (b *BadWordUtil) ReplaceBadWordsInBytes(input []byte) ([]byte, error) {
	if b.Matcher == nil {
		b.ByteMatcher = CompileByteSlices(b.BadWordsListByte)
		return input, errors.New("bad words matcher not initialized")
	}

	mask := make([]bool, len(input))
	matchesFound := false

	b.ByteMatcher.Iterate(input, func(word []byte, start int) bool {
		matchesFound = true
		end := start + len(word)
		for k := start; k < end; k++ {
			mask[k] = true
		}
		return true
	})

	if !matchesFound {
		return input, nil
	}

	var output []byte
	// output = make([]byte, 0, len(input))

	i := 0
	for i < len(input) {
		if mask[i] {
			_, size := utf8.DecodeRune(input[i:])
			output = append(output, '*')
			i += size
		} else {
			_, size := utf8.DecodeRune(input[i:])
			output = append(output, input[i:i+size]...)
			i += size
		}
	}

	return output, nil
}

func (b *BadWordUtil) ProcessChatMessage(message []byte) ([]byte, error) {
	// Remove URLs directly from []byte
	message = RemoveURLsFromBytes(message)

	// Replace bad words directly in []byte
	return b.ReplaceBadWordsInBytes(message)
}

func RemoveURLs(input string) string {
	// Replace URLs with an empty string
	return urlRegex.ReplaceAllString(input, "")
}

func RemoveURLsFromBytes(message []byte) []byte {
	return urlRegex.ReplaceAll(message, []byte(""))
}

// LoadBadWords loads bad words from a file into memory with optimizations.
func (b *BadWordUtil) LoadBadWords(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Estimate the number of words if known or use a high number.
	const estimatedWords = 1000
	b.BadWordsList = make([]string, 0, estimatedWords)

	// Create a buffer and attach it to scanner.
	scanner := bufio.NewScanner(file)
	const maxCapacity = 10 * 1024 // 10KB;
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		word := scanner.Text()
		b.BadWordsList = append(b.BadWordsList, word)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Optimize memory usage by shrinking the slice to the actual number of words.
	b.BadWordsList = append([]string{}, b.BadWordsList...)

	// Compile the list of bad words into a trie (Aho-corasick Double-Array Trie)
	b.Matcher = CompileStrings(b.BadWordsList)
	return nil
}

func (b *BadWordUtil) LoadBadWordsByte(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	const estimatedWords = 600
	b.BadWordsListByte = make([][]byte, 0, estimatedWords)

	scanner := bufio.NewScanner(file)
	const maxCapacity = 10 * 1024 // 10KB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		word := make([]byte, len(scanner.Bytes()))
		copy(word, scanner.Bytes())
		b.BadWordsListByte = append(b.BadWordsListByte, word)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	b.ByteMatcher = CompileByteSlices(b.BadWordsListByte)
	return nil
}

// CheckForBadWordsUsingTrie checks if the input contains any bad words using Aho-Corasick trie
func (b *BadWordUtil) CheckForBadWordsUsingTrie(input string) (bool, error) {
	if b.Matcher == nil {
		b.Matcher = CompileStrings(b.BadWordsList)
		return false, os.ErrNotExist
	}
	found := false
	b.Matcher.IterateString(input, func(word string, start int) bool {
		found = true
		return false // Start after first match
	})
	return found, nil
}

// BytesToString converts a byte slice to a string without making a copy.
func BytesToString(b []byte) string {
	return unsafe.String(&b[0], len(b))
}

// StringToBytes converts a string to a byte slice without making a copy.
func StringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// Precompute rune indices for the whole string to avoid recalculating repeatedly
func computeRuneIndices(input string) []int {
	runeIndices := make([]int, len(input)+1)
	runeCount := 0
	for i := range input {
		runeIndices[i] = runeCount
		_, size := utf8.DecodeRuneInString(input[i:])
		runeCount += 1
		i += size - 1 // Adjust for the size of the current rune
	}
	runeIndices[len(input)] = runeCount // End of string index
	return runeIndices
}

// SliceFromPointer creates a slice from a pointer and a length.
func SliceFromPointer[T any](base unsafe.Pointer, length int) []T {
	return unsafe.Slice((*T)(base), length)
}
