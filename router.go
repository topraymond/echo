package echo

import "net/http"

type (
	router struct {
		trees map[string]*node
		echo  *Echo
	}
	node struct {
		label   byte
		prefix  string
		has     ntype // Type of node it contains
		handler HandlerFunc
		eid     uint8 // Echo id
		edges   edges
	}
	edges []*node
	ntype byte
	param struct {
		Name  string
		Value string
	}
	Params []param
)

const (
	snode ntype = iota // Static node
	pnode              // Param node
	anode              // Catch-all node
)

func NewRouter(e *Echo) (r *router) {
	r = &router{
		trees: make(map[string]*node),
		echo:  e,
	}
	for _, m := range methods {
		r.trees[m] = &node{
			prefix: "",
			edges:  edges{},
		}
	}
	return
}

func (r *router) Add(method, path string, h HandlerFunc, eid uint8) {
	i := 0
	l := len(path)
	for ; i < l; i++ {
		if path[i] == ':' {
			r.insert(method, path[:i], nil, eid, pnode)
			for ; i < l && path[i] != '/'; i++ {
			}
			if i == l {
				r.insert(method, path[:i], h, eid, snode)
				return
			}
			r.insert(method, path[:i], nil, eid, snode)
		} else if path[i] == '*' {
			r.insert(method, path[:i], h, eid, anode)
		}
	}
	r.insert(method, path, h, eid, snode)
}

func (r *router) insert(method, path string, h HandlerFunc, eid uint8, has ntype) {
	cn := r.trees[method] // Current node as root
	search := path

	for {
		sl := len(search)
		pl := len(cn.prefix)
		l := lcp(search, cn.prefix)

		if l == 0 {
			// At root node
			cn.label = search[0]
			cn.prefix = search
			cn.has = has
			if h != nil {
				cn.handler = h
				cn.eid = eid
			}
			return
		} else if l < pl {
			// Split the node
			n := newNode(cn.prefix[l:], cn.has, cn.handler, cn.eid, cn.edges)
			cn.edges = edges{n} // Add to parent

			// Reset parent node
			cn.label = cn.prefix[0]
			cn.prefix = cn.prefix[:l]
			cn.has = snode
			cn.handler = nil
			cn.eid = 0

			if l == sl {
				// At parent node
				cn.handler = h
				cn.eid = eid
			} else {
				// Need to fork a node
				n = newNode(search[l:], has, h, eid, edges{})
				cn.edges = append(cn.edges, n)
			}
			break
		} else if l < sl {
			search = search[l:]
			e := cn.findEdge(search[0])
			if e == nil {
				n := newNode(search, has, h, eid, edges{})
				cn.edges = append(cn.edges, n)
				break
			} else {
				cn = e
			}
		} else {
			// Node already exists
			if h != nil {
				cn.handler = h
				cn.eid = eid
			}
			break
		}
	}
}

func newNode(pfx string, has ntype, h HandlerFunc, eid uint8, e edges) (n *node) {
	n = &node{
		label:   pfx[0],
		prefix:  pfx,
		has:     has,
		handler: h,
		eid:     eid,
		edges:   e,
	}
	return
}

func (n *node) findEdge(l byte) *node {
	for _, e := range n.edges {
		if e.label == l {
			return e
		}
	}
	return nil
}

// Length of longest common prefix
func lcp(a, b string) (i int) {
	max := len(a)
	l := len(b)
	if l < max {
		max = l
	}
	for ; i < max && a[i] == b[i]; i++ {
	}
	return
}

func (r *router) Find(method, path string) (h HandlerFunc, c *Context, eid uint8) {
	c = r.echo.pool.Get().(*Context)
	cn := r.trees[method] // Current node as root
	search := path
	n := 0 // Param count

	for {
		if search == "" || search == cn.prefix {
			h = cn.handler
			eid = cn.eid
			return
		}

		pl := len(cn.prefix)
		l := lcp(search, cn.prefix)

		if l == pl {
			search = search[l:]
			switch cn.has {
			case pnode:
				cn = cn.edges[0]
				i := 0
				l = len(search)

				for ; i < l && search[i] != '/'; i++ {
				}
				p := c.params[:n+1]
				p[n].Name = cn.prefix[1:]
				p[n].Value = search[:i]
				n++

				search = search[i:]

				if i == l {
					// All params read
					continue
				}
			case anode:
				p := c.params[:n+1]
				p[n].Name = "_name"
				p[n].Value = search
				search = "" // End search
				continue
			}
			e := cn.findEdge(search[0])
			if e == nil {
				// Not found
				return
			}
			cn = e
			continue
		} else {
			// Not found
			return
		}
	}
}

// Get returns path parameter by name.
func (ps Params) Get(n string) (v string) {
	for _, p := range ps {
		if p.Name == n {
			v = p.Value
		}
	}
	return
}

func (r *router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h, c, _ := r.Find(req.Method, req.URL.Path)
	c.Response.ResponseWriter = rw
	if h != nil {
		h(c)
	} else {
		r.echo.notFoundHandler(c)
	}
	r.echo.pool.Put(c)
}
