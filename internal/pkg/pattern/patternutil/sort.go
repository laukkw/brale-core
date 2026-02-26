package patternutil

import "brale-core/internal/pkg/numutil"

func SortByScoreIndexName[T any](items []T, score func(T) int, index func(T) int, name func(T) string) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			ai := numutil.AbsInt(score(items[i]))
			aj := numutil.AbsInt(score(items[j]))
			if aj > ai || (aj == ai && (index(items[j]) > index(items[i]) || (index(items[j]) == index(items[i]) && name(items[j]) < name(items[i])))) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}
