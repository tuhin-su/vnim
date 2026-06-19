package tree

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tuhin-su/vnim/pkg/config"
)

type Node struct {
	Name     string
	Type     string
	Details  string
	Children []*Node
}

func (n *Node) Label() string {
	var sb strings.Builder
	sb.WriteString(n.Name)
	if n.Type != "" {
		sb.WriteString(fmt.Sprintf(" (%s)", n.Type))
	}
	if n.Details != "" {
		sb.WriteString(fmt.Sprintf(" [%s]", n.Details))
	}
	return sb.String()
}

func BuildTree(cfg *config.Config) []*Node {
	nodes := make(map[string]*Node)

	// Step 1: Create all nodes
	for idx, obj := range cfg.Objects {
		if obj.Type == "address" {
			continue
		}

		var node *Node
		var nodeKey string

		if obj.Type == "route" {
			dest := obj.Destination
			if dest == "" {
				dest = "0.0.0.0/0 (default)"
			}
			devSuffix := ""
			if obj.Interface != "" {
				devSuffix = fmt.Sprintf(" dev %s", obj.Interface)
			}
			node = &Node{
				Name:    fmt.Sprintf("route to %s", dest),
				Type:    "route",
				Details: fmt.Sprintf("via %s%s", obj.Gateway, devSuffix),
			}
			nodeKey = fmt.Sprintf("route-%d", idx)
		} else if obj.Type == "nat" {
			node = &Node{
				Name:    fmt.Sprintf("nat masquerade on %s", obj.Interface),
				Type:    "nat",
				Details: fmt.Sprintf("for %s", obj.SourceSubnet),
			}
			nodeKey = fmt.Sprintf("nat-%d", idx)
		} else {
			node = &Node{
				Name:     obj.Name,
				Type:     obj.Type,
				Children: make([]*Node, 0),
			}
			nodeKey = obj.Name

			if obj.Type == "veth" && obj.Peer != "" {
				node.Details = fmt.Sprintf("peer: %s", obj.Peer)
			} else if obj.Type == "vlan" {
				node.Details = fmt.Sprintf("id: %d, parent: %s", obj.VlanID, obj.Parent)
			} else if obj.Type == "vxlan" {
				vni := obj.VxlanID
				if vni == 0 {
					vni = obj.VlanID
				}
				node.Details = fmt.Sprintf("id: %d, parent: %s", vni, obj.Parent)
				if obj.Group != "" {
					node.Details += fmt.Sprintf(", group: %s", obj.Group)
				}
			}

			if obj.Mac != "" {
				if node.Details != "" {
					node.Details += ", mac: " + obj.Mac
				} else {
					node.Details = "mac: " + obj.Mac
				}
			}
		}

		nodes[nodeKey] = node
	}

	// Add services to interface nodes
	for _, svc := range cfg.Services {
		if node, ok := nodes[svc.Interface]; ok {
			svcDetail := fmt.Sprintf("svc: %s", svc.Type)
			if svc.Type == "plugin" {
				if svc.Path != "" {
					svcDetail += fmt.Sprintf(" (%s)", filepath.Base(svc.Path))
				} else {
					svcDetail += " (inline)"
				}
			}
			if svc.Port > 0 {
				svcDetail += fmt.Sprintf(":%d", svc.Port)
			}
			if node.Details != "" {
				node.Details += ", " + svcDetail
			} else {
				node.Details = svcDetail
			}
		}
	}

	// Add IP addresses to interface nodes
	for _, obj := range cfg.Objects {
		if obj.Type == "address" {
			if node, ok := nodes[obj.Interface]; ok {
				if node.Details != "" {
					node.Details += ", ip: " + obj.Address
				} else {
					node.Details = "ip: " + obj.Address
				}
			}
		}
	}

	// Step 2: Establish hierarchy
	roots := make([]*Node, 0)
	hasParent := make(map[string]bool)

	for idx, obj := range cfg.Objects {
		if obj.Type == "address" {
			continue
		}

		var nodeKey string
		if obj.Type == "route" {
			nodeKey = fmt.Sprintf("route-%d", idx)
		} else if obj.Type == "nat" {
			nodeKey = fmt.Sprintf("nat-%d", idx)
		} else {
			nodeKey = obj.Name
		}

		node := nodes[nodeKey]
		if node == nil {
			continue
		}

		var parentNode *Node

		if obj.Type == "namespace" {
			continue
		}

		if obj.Type == "route" {
			if obj.Namespace != "" {
				parentNode = nodes[obj.Namespace]
			}
		} else if obj.Type == "nat" {
			// NAT rules are root level
		} else {
			// Rule 2: If attached to a bridge, parent is the bridge
			if obj.Master != "" {
				parentNode = nodes[obj.Master]
			} else if obj.Parent != "" && (obj.Type == "vlan" || obj.Type == "vxlan") {
				// Rule 3: If vlan/vxlan, parent is parent interface
				parentNode = nodes[obj.Parent]
			} else if obj.Namespace != "" {
				// Rule 4: If in namespace, parent is namespace
				parentNode = nodes[obj.Namespace]
			}
		}

		if parentNode != nil {
			parentNode.Children = append(parentNode.Children, node)
			hasParent[nodeKey] = true
		}
	}

	// Any node that does not have a parent is a root node
	for idx, obj := range cfg.Objects {
		if obj.Type == "address" {
			continue
		}
		var nodeKey string
		if obj.Type == "route" {
			nodeKey = fmt.Sprintf("route-%d", idx)
		} else if obj.Type == "nat" {
			nodeKey = fmt.Sprintf("nat-%d", idx)
		} else {
			nodeKey = obj.Name
		}

		if !hasParent[nodeKey] {
			if node, ok := nodes[nodeKey]; ok {
				roots = append(roots, node)
			}
		}
	}

	return roots
}

func RenderTree(roots []*Node) string {
	var sb strings.Builder
	for i, root := range roots {
		isLast := i == len(roots)-1
		renderNode(&sb, root, "", isLast)
	}
	return sb.String()
}

func renderNode(sb *strings.Builder, n *Node, prefix string, isLast bool) {
	if n == nil {
		return
	}

	marker := "├── "
	if isLast {
		marker = "└── "
	}

	sb.WriteString(prefix + marker + n.Label() + "\n")

	nextPrefix := prefix + "│   "
	if isLast {
		nextPrefix = prefix + "    "
	}

	for i, child := range n.Children {
		isLastChild := i == len(n.Children)-1
		renderNode(sb, child, nextPrefix, isLastChild)
	}
}
