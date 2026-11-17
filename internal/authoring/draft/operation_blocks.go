package draft

import "github.com/fuseone/agents/internal/authoring/blocks"

// The block operations change the instructions, by stable block id, and come
// only from an accepted proposal (contract 3). A changed block is written
// under its kind's label in Locale, the locale of whoever accepted it.

// InsertBlock adds a block of a type at a position.
type InsertBlock struct {
	At           int
	Type         blocks.Kind
	Text, Locale string
}

func (InsertBlock) Kind() string     { return "insert_block" }
func (InsertBlock) Class() Class     { return Conversation }
func (InsertBlock) Fields() []string { return []string{"instructions"} }
func (o InsertBlock) apply(d *Draft) error {
	return onBlocks(d, o.Kind(), func(b []blocks.Block) ([]blocks.Block, error) {
		return blocks.Insert(b, o.At, o.Type, o.Text, o.Locale)
	})
}

// ReplaceBlock sets a block's text.
type ReplaceBlock struct{ ID, Text, Locale string }

func (ReplaceBlock) Kind() string     { return "replace_block" }
func (ReplaceBlock) Class() Class     { return Conversation }
func (ReplaceBlock) Fields() []string { return []string{"instructions"} }
func (o ReplaceBlock) apply(d *Draft) error {
	return onBlocks(d, o.Kind(), func(b []blocks.Block) ([]blocks.Block, error) {
		return blocks.Replace(b, o.ID, o.Text, o.Locale)
	})
}

// RemoveBlock takes a block out.
type RemoveBlock struct{ ID string }

func (RemoveBlock) Kind() string     { return "remove_block" }
func (RemoveBlock) Class() Class     { return Conversation }
func (RemoveBlock) Fields() []string { return []string{"instructions"} }
func (o RemoveBlock) apply(d *Draft) error {
	return onBlocks(d, o.Kind(), func(b []blocks.Block) ([]blocks.Block, error) {
		return blocks.Remove(b, o.ID)
	})
}

// MoveBlock puts a block at a position.
type MoveBlock struct {
	ID string
	To int
}

func (MoveBlock) Kind() string     { return "move_block" }
func (MoveBlock) Class() Class     { return Conversation }
func (MoveBlock) Fields() []string { return []string{"instructions"} }
func (o MoveBlock) apply(d *Draft) error {
	return onBlocks(d, o.Kind(), func(b []blocks.Block) ([]blocks.Block, error) {
		return blocks.Move(b, o.ID, o.To)
	})
}

func onBlocks(d *Draft, kind string, change func([]blocks.Block) ([]blocks.Block, error)) error {
	out, err := change(d.Blocks)
	if err != nil {
		return invalidOp(kind, "%v", err)
	}
	d.Blocks = out
	return nil
}
