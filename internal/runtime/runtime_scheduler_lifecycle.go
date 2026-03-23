package runtime

import "context"

func (s *RuntimeScheduler) lifecycleSnapshot() (bool, context.Context) {
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	return s.started, s.streamCtx
}

func (s *RuntimeScheduler) setLifecycle(started bool, streamCtx context.Context, cancel context.CancelFunc) {
	s.lifecycleMu.Lock()
	s.started = started
	s.streamCtx = streamCtx
	s.cancel = cancel
	s.lifecycleMu.Unlock()
}

func (s *RuntimeScheduler) lifecycleStopSnapshot() (bool, context.Context, context.CancelFunc) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	started := s.started
	streamCtx := s.streamCtx
	cancel := s.cancel
	s.started = false
	s.streamCtx = nil
	s.cancel = nil
	return started, streamCtx, cancel
}
