package grpcadapter

import (
	"errors"
	"fmt"

	"github.com/KucherenkoIvan/go-kernel/grpckit"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// mapRemoteError turns wire errors back into the typed domain errors the
// faces already check with errors.As — remote mode stays behaviorally
// identical to local.
func mapRemoteError(err error) error {
	if err == nil {
		return nil
	}

	var remote *grpckit.RemoteDomainError
	if errors.As(err, &remote) {
		switch remote.Code {
		case (&domain.MemoryNotFoundError{}).Error():
			return &domain.MemoryNotFoundError{}
		case (&domain.MemorySupersededError{}).Error():
			return &domain.MemorySupersededError{}
		case (&domain.InvalidSupersedeError{}).Error():
			return &domain.InvalidSupersedeError{}
		case (&domain.EmptyContentError{}).Error():
			return &domain.EmptyContentError{}
		case (&domain.InvalidSummaryError{}).Error():
			return &domain.InvalidSummaryError{}
		case (&domain.InvalidKindError{}).Error():
			return &domain.InvalidKindError{}
		case (&domain.NoKeywordsError{}).Error():
			return &domain.NoKeywordsError{}
		case (&domain.EmptySourceError{}).Error():
			return &domain.EmptySourceError{}
		case (&domain.InvalidTTLError{}).Error():
			return &domain.InvalidTTLError{}
		}
		return remote // unknown domain code: keep the snake_code error text
	}

	if st, ok := status.FromError(err); ok && st.Code() == codes.Unauthenticated {
		return fmt.Errorf("remote memory rejected the API key — check `recall remote status`: %s", st.Message())
	}
	return err
}
