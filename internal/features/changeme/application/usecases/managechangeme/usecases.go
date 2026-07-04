package managechangeme

// UseCases bundles the feature's application surface — what every transport
// adapter (rest, grpc, tui) is built from.
type UseCases struct {
	Create *CreateCommand
	Update *UpdateCommand
	Delete *DeleteCommand
	Get    *GetQuery
	List   *ListQuery
}
