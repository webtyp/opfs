package opfs

// splitPath splits a relative path on "/". It refuses "", a leading or trailing "/", an empty
// segment ("a//b"), "." and "..".
func splitPath(path string) ([]string, bool) {
	if len(path) == 0 || path[0] == '/' || path[len(path)-1] == '/' {
		return nil, false
	}

	// Count segments to allocate slice
	count := 1
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			count++
		}
	}

	segments := make([]string, 0, count)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			seg := path[start:i]
			if len(seg) == 0 || seg == "." || seg == ".." {
				return nil, false
			}
			segments = append(segments, seg)
			start = i + 1
		}
	}

	return segments, true
}
