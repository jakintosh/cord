package topology

// Topology is a compiled snapshot whose tree and resolver share one
// containment index.
type Topology struct {
	tree     *Tree
	resolver *Resolver
}

// New compiles a snapshot for hierarchy inspection and visibility resolution.
func New(
	s *Snapshot,
) (
	*Topology,
	error,
) {
	containment, err := buildContainment(s.Cidrs)
	if err != nil {
		return nil, err
	}

	return &Topology{
		tree:     newTree(s, containment),
		resolver: newResolver(s, containment),
	}, nil
}

func (t *Topology) Tree() *Tree {
	return t.tree
}

func (t *Topology) Resolver() *Resolver {
	return t.resolver
}
