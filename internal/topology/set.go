package topology

import "slices"

type stringSet map[string]struct{}

func newStringSet(
	values ...string,
) stringSet {
	set := make(stringSet, len(values))
	for _, value := range values {
		set.add(value)
	}
	return set
}

func stringSetFromBoolMap(
	values map[string]bool,
) stringSet {
	set := make(stringSet, len(values))
	for value, included := range values {
		if included {
			set.add(value)
		}
	}
	return set
}

func (s stringSet) add(
	value string,
) {
	s[value] = struct{}{}
}

func (s stringSet) contains(
	value string,
) bool {
	_, ok := s[value]
	return ok
}

func (s stringSet) intersects(
	other stringSet,
) bool {
	for value := range s {
		if other.contains(value) {
			return true
		}
	}
	return false
}

func (s stringSet) clone() stringSet {
	result := make(stringSet, len(s))
	for value := range s {
		result.add(value)
	}
	return result
}

func (s stringSet) sorted() []string {
	result := make([]string, 0, len(s))
	for value := range s {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func (s stringSet) boolMap() map[string]bool {
	result := make(map[string]bool, len(s))
	for value := range s {
		result[value] = true
	}
	return result
}
