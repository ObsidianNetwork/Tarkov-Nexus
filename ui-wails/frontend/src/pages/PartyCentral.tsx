import React, { useState, useEffect, useCallback } from 'react';
import {
  ConnectToPartyServer,
  DisconnectFromPartyServer,
  CreateCentralParty,
  JoinCentralParty,
  LeaveCentralParty,
  GetCentralPartyStatus,
  GetCentralPartyMembers,
  GetFriends,
  AddFriend,
  RemoveFriend,
  InviteFriendToParty,
  AcceptPartyInvite,
  DeclinePartyInvite,
  CancelPartyInvite,
  GetPendingInvites,
  AcceptFriendRequest,
  DeclineFriendRequest,
  CancelFriendRequest,
  GetPendingFriendRequests,
  GetIncomingFriendRequests,
  GetOutgoingFriendRequests,
  GetConfig,
} from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { Button, Card } from '../components/ui';
import {
  UserGroupIcon,
  ClipboardDocumentIcon,
  CheckIcon,
  XMarkIcon,
  SignalIcon,
  MapPinIcon,
  UserPlusIcon,
  UserMinusIcon,
  BellIcon,
  ArrowRightOnRectangleIcon,
  PlusIcon,
} from '@heroicons/react/24/outline';
import type { CentralPartyStatus, PartyMemberInfo, Friend, PartyInvite, Position } from '../types';

interface FriendRequest {
  clientId: string;
  displayName: string;
  timestamp?: string;
  sentAt?: string;
}

export default function PartyCentral() {
  const [status, setStatus] = useState<CentralPartyStatus>({
    connected: false,
    registered: false,
    inParty: false,
    partyCode: '',
    isHost: false,
    memberCount: 0,
    serverUrl: '',
    clientId: '',
  });
  const [members, setMembers] = useState<PartyMemberInfo[]>([]);
  const [friends, setFriends] = useState<Friend[]>([]);
  const [pendingInvites, setPendingInvites] = useState<PartyInvite[]>([]);
  const [incomingFriendRequests, setIncomingFriendRequests] = useState<FriendRequest[]>([]);
  const [outgoingFriendRequests, setOutgoingFriendRequests] = useState<FriendRequest[]>([]);
  const [joinCode, setJoinCode] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [copied, setCopied] = useState(false);
  const [activeTab, setActiveTab] = useState<'party' | 'friends'>('party');
  const [sentInvites, setSentInvites] = useState<Set<string>>(new Set());
  const [displayName, setDisplayName] = useState('Player');

  const refreshStatus = useCallback(async () => {
    try {
      const partyStatus = await GetCentralPartyStatus();
      setStatus(partyStatus as CentralPartyStatus);

      if (partyStatus.inParty) {
        const partyMembers = await GetCentralPartyMembers();
        setMembers(partyMembers as PartyMemberInfo[]);
      }

      if (partyStatus.registered) {
        const friendsList = await GetFriends();
        if (friendsList) {
          setFriends(friendsList as Friend[]);
        }
        const invites = await GetPendingInvites();
        if (invites) {
          setPendingInvites(invites as PartyInvite[]);
        }
        // Fetch friend requests
        const incoming = await GetIncomingFriendRequests();
        if (incoming) {
          setIncomingFriendRequests(incoming as FriendRequest[]);
        }
        const outgoing = await GetOutgoingFriendRequests();
        if (outgoing) {
          setOutgoingFriendRequests(outgoing as FriendRequest[]);
        }
        // Also request fresh data from server
        await GetPendingFriendRequests().catch(() => {});
      }
    } catch (err) {
      console.error('Failed to refresh status:', err);
    }
  }, []);

  // Load display name from config
  useEffect(() => {
    GetConfig().then((cfg: any) => {
      if (cfg?.partySettings?.displayName) {
        setDisplayName(cfg.partySettings.displayName);
      }
    }).catch(() => {});
  }, []);

  useEffect(() => {
    refreshStatus();
    const interval = setInterval(refreshStatus, 3000);

    // Event listeners
    const cleanups = [
      EventsOn('centralParty:connected', refreshStatus),
      EventsOn('centralParty:disconnected', refreshStatus),
      EventsOn('centralParty:registered', refreshStatus),
      EventsOn('centralParty:created', refreshStatus),
      EventsOn('centralParty:joined', refreshStatus),
      EventsOn('centralParty:left', refreshStatus),
      EventsOn('centralParty:memberJoined', refreshStatus),
      EventsOn('centralParty:memberLeft', refreshStatus),
      EventsOn('centralParty:positionUpdate', (data: any) => {
        setMembers(prev => prev.map(m =>
          m.clientId === data.clientId
            ? { ...m, currentMap: data.map, position: data.position as Position }
            : m
        ));
      }),
      EventsOn('centralParty:friendsList', (data: Friend[]) => setFriends(data)),
      EventsOn('centralParty:friendOnline', refreshStatus),
      EventsOn('centralParty:friendOffline', refreshStatus),
      EventsOn('centralParty:friendAdded', refreshStatus),
      EventsOn('centralParty:friendRemoved', refreshStatus),
      EventsOn('centralParty:invite', (data: PartyInvite) => {
        setPendingInvites(prev => [...prev, data]);
      }),
      EventsOn('centralParty:error', (data: { code: string; message: string }) => {
        setError(data.message);
        setTimeout(() => setError(''), 5000);
      }),
      EventsOn('centralParty:inviteCancelled', (data: { fromClientId: string; partyCode: string }) => {
        // Remove the cancelled invite from the pending list
        setPendingInvites(prev => prev.filter(i => i.partyCode !== data.partyCode));
      }),
      EventsOn('centralParty:inviteAccepted', (data: { clientId: string }) => {
        // Remove from sent invites when accepted
        setSentInvites(prev => {
          const newSet = new Set(prev);
          newSet.delete(data.clientId);
          return newSet;
        });
      }),
      EventsOn('centralParty:inviteDeclined', (data: { clientId: string }) => {
        // Remove from sent invites when declined
        setSentInvites(prev => {
          const newSet = new Set(prev);
          newSet.delete(data.clientId);
          return newSet;
        });
      }),
      EventsOn('centralParty:left', () => {
        // Clear sent invites when leaving party
        setSentInvites(new Set());
      }),
      // Friend request events
      EventsOn('centralParty:friendRequest', (data: FriendRequest) => {
        setIncomingFriendRequests(prev => {
          // Avoid duplicates
          if (prev.some(r => r.clientId === data.clientId)) return prev;
          return [...prev, data];
        });
      }),
      EventsOn('centralParty:friendRequestSent', (data: FriendRequest) => {
        setOutgoingFriendRequests(prev => {
          // Avoid duplicates
          if (prev.some(r => r.clientId === data.clientId)) return prev;
          return [...prev, data];
        });
      }),
      EventsOn('centralParty:friendRequestAccepted', (data: { clientId: string }) => {
        // Remove from outgoing requests
        setOutgoingFriendRequests(prev => prev.filter(r => r.clientId !== data.clientId));
      }),
      EventsOn('centralParty:friendRequestDeclined', (data: { clientId: string }) => {
        // Remove from outgoing requests
        setOutgoingFriendRequests(prev => prev.filter(r => r.clientId !== data.clientId));
      }),
      EventsOn('centralParty:friendRequestCancelled', (data: { fromClientId: string }) => {
        // Remove from incoming requests
        setIncomingFriendRequests(prev => prev.filter(r => r.clientId !== data.fromClientId));
      }),
      EventsOn('centralParty:friendRequestsList', (data: { incoming: FriendRequest[]; outgoing: FriendRequest[] }) => {
        setIncomingFriendRequests(data.incoming || []);
        setOutgoingFriendRequests(data.outgoing || []);
      }),
    ];

    return () => {
      clearInterval(interval);
      cleanups.forEach(cleanup => {
        if (typeof cleanup === 'function') cleanup();
      });
    };
  }, [refreshStatus]);

  const handleConnect = async () => {
    setIsLoading(true);
    setError('');
    try {
      // Pass display name directly — Go side saves it to config
      await ConnectToPartyServer(displayName.trim() || 'Player');
    } catch (err: any) {
      setError(err.message || 'Failed to connect');
    } finally {
      setIsLoading(false);
    }
  };

  const handleDisconnect = async () => {
    setIsLoading(true);
    try {
      await DisconnectFromPartyServer();
    } catch (err: any) {
      setError(err.message || 'Failed to disconnect');
    } finally {
      setIsLoading(false);
    }
  };

  const handleCreateParty = async () => {
    setIsLoading(true);
    setError('');
    try {
      await CreateCentralParty();
    } catch (err: any) {
      setError(err.message || 'Failed to create party');
    } finally {
      setIsLoading(false);
    }
  };

  const handleJoinParty = async () => {
    if (!joinCode.trim()) {
      setError('Please enter a party code');
      return;
    }
    setIsLoading(true);
    setError('');
    try {
      await JoinCentralParty(joinCode.trim().toUpperCase());
      setJoinCode('');
    } catch (err: any) {
      setError(err.message || 'Failed to join party');
    } finally {
      setIsLoading(false);
    }
  };

  const handleLeaveParty = async () => {
    setIsLoading(true);
    try {
      await LeaveCentralParty();
    } catch (err: any) {
      setError(err.message || 'Failed to leave party');
    } finally {
      setIsLoading(false);
    }
  };

  const handleCopyCode = () => {
    navigator.clipboard.writeText(status.partyCode);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleInviteFriend = async (friendId: string) => {
    try {
      await InviteFriendToParty(friendId);
      // Track that we sent an invite to this friend
      setSentInvites(prev => new Set(prev).add(friendId));
    } catch (err: any) {
      setError(err.message || 'Failed to invite friend');
    }
  };

  const handleCancelInvite = async (friendId: string) => {
    try {
      await CancelPartyInvite(friendId);
      // Remove from sent invites
      setSentInvites(prev => {
        const newSet = new Set(prev);
        newSet.delete(friendId);
        return newSet;
      });
    } catch (err: any) {
      setError(err.message || 'Failed to cancel invite');
    }
  };

  const handleAcceptInvite = async (partyCode: string) => {
    setIsLoading(true);
    setError('');
    try {
      await AcceptPartyInvite(partyCode);
      setPendingInvites(prev => prev.filter(i => i.partyCode !== partyCode));
    } catch (err: any) {
      setError(err.message || 'Failed to accept invite');
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeclineInvite = async (partyCode: string) => {
    try {
      await DeclinePartyInvite(partyCode);
      setPendingInvites(prev => prev.filter(i => i.partyCode !== partyCode));
    } catch (err: any) {
      setError(err.message || 'Failed to decline invite');
    }
  };

  const handleAddFriend = async (clientId: string) => {
    try {
      await AddFriend(clientId);
    } catch (err: any) {
      setError(err.message || 'Failed to send friend request');
    }
  };

  const handleRemoveFriend = async (clientId: string) => {
    try {
      await RemoveFriend(clientId);
    } catch (err: any) {
      setError(err.message || 'Failed to remove friend');
    }
  };

  const handleAcceptFriendRequest = async (clientId: string) => {
    try {
      await AcceptFriendRequest(clientId);
      setIncomingFriendRequests(prev => prev.filter(r => r.clientId !== clientId));
    } catch (err: any) {
      setError(err.message || 'Failed to accept friend request');
    }
  };

  const handleDeclineFriendRequest = async (clientId: string) => {
    try {
      await DeclineFriendRequest(clientId);
      setIncomingFriendRequests(prev => prev.filter(r => r.clientId !== clientId));
    } catch (err: any) {
      setError(err.message || 'Failed to decline friend request');
    }
  };

  const handleCancelFriendRequest = async (clientId: string) => {
    try {
      await CancelFriendRequest(clientId);
      setOutgoingFriendRequests(prev => prev.filter(r => r.clientId !== clientId));
    } catch (err: any) {
      setError(err.message || 'Failed to cancel friend request');
    }
  };

  // Not connected to server
  if (!status.connected) {
    return (
      <div className="min-h-full p-8 animate-fade-in-up">
        <div className="mb-8">
          <h1 className="text-4xl font-bold gradient-text mb-2">Party</h1>
          <p className="text-text-secondary">Squad up with friends and share positions in real-time</p>
        </div>

        <div className="max-w-lg">
          <Card className="p-8">
            <div className="text-center mb-8">
              <div className="w-16 h-16 rounded-full bg-primary-purple/20 flex items-center justify-center mx-auto mb-4">
                <SignalIcon className="h-8 w-8 text-primary-purple" />
              </div>
              <h2 className="text-xl font-bold text-text-primary mb-2">Connect to Party Server</h2>
              <p className="text-text-secondary text-sm">
                Set your display name and connect to create or join parties.
              </p>
            </div>

            <div className="space-y-4 mb-6">
              <div>
                <label className="block text-sm font-medium text-text-secondary mb-2">Display Name</label>
                <input
                  type="text"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder="Enter your name"
                  className="glass-input w-full px-4 py-3 text-lg"
                />
                <p className="text-xs text-text-muted mt-1">This is how other players will see you</p>
              </div>
            </div>

            {error && (
              <div className="bg-red-900/30 border border-red-700 rounded p-3 mb-4">
                <p className="text-red-400 text-sm">{error}</p>
              </div>
            )}

            <Button
              onClick={handleConnect}
              disabled={isLoading || !displayName.trim()}
              loading={isLoading}
              className="w-full bg-neon-green hover:bg-neon-green/90 hover:shadow-glow-green"
              size="lg"
            >
              {isLoading ? 'Connecting...' : 'Connect to Server'}
            </Button>
          </Card>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <UserGroupIcon className="h-7 w-7" />
          Party
        </h1>

        <div className="flex items-center gap-3">
          {/* Connection Status */}
          <div className="flex items-center gap-2 text-sm">
            <span className="relative flex h-2 w-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2 w-2 bg-green-500"></span>
            </span>
            <span className="text-green-400">Connected</span>
          </div>

          <Button variant="secondary" size="sm" onClick={handleDisconnect}>
            Disconnect
          </Button>
        </div>
      </div>

      {/* Pending Invites */}
      {pendingInvites.length > 0 && (
        <div className="bg-blue-900/30 border border-blue-700 rounded-lg p-4 mb-6">
          <h3 className="font-semibold flex items-center gap-2 mb-3">
            <BellIcon className="h-5 w-5" />
            Party Invites ({pendingInvites.length})
          </h3>
          <div className="space-y-2">
            {pendingInvites.map((invite, i) => (
              <div key={i} className="flex items-center justify-between glass-panel p-3">
                <div>
                  <span className="font-medium">{invite.fromDisplayName}</span>
                  <span className="text-text-secondary text-sm ml-2">invited you to party</span>
                </div>
                <div className="flex gap-2">
                  <Button size="sm" onClick={() => handleAcceptInvite(invite.partyCode)} disabled={isLoading} title="Accept party invite">
                    <CheckIcon className="h-4 w-4" />
                    Accept
                  </Button>
                  <Button size="sm" variant="secondary" onClick={() => handleDeclineInvite(invite.partyCode)} disabled={isLoading} title="Decline party invite">
                    <XMarkIcon className="h-4 w-4" />
                    Decline
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {error && (
        <div className="bg-red-900/30 border border-red-700 rounded p-3 mb-4">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 mb-6 glass-panel p-1 max-w-xs">
        <button
          onClick={() => setActiveTab('party')}
          className={`flex-1 py-2 px-4 rounded-md text-sm font-medium transition ${
            activeTab === 'party'
              ? 'bg-primary-purple text-white'
              : 'text-text-muted hover:text-text-primary'
          }`}
        >
          Party
        </button>
        <button
          onClick={() => setActiveTab('friends')}
          className={`flex-1 py-2 px-4 rounded-md text-sm font-medium transition ${
            activeTab === 'friends'
              ? 'bg-primary-purple text-white'
              : 'text-text-muted hover:text-text-primary'
          }`}
        >
          Friends ({friends.filter(f => f.online).length}/{friends.length})
        </button>
      </div>

      {activeTab === 'party' && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Left: Party Controls */}
          <div className="space-y-6">
            {!status.inParty ? (
              <>
                {/* Create Party */}
                <Card>
                  <h2 className="text-lg font-semibold mb-4">Create a Party</h2>
                  <p className="text-text-secondary text-sm mb-4">
                    Create a new party and share the code with your squad.
                  </p>
                  <Button onClick={handleCreateParty} disabled={isLoading} className="w-full">
                    <PlusIcon className="h-5 w-5 mr-2" />
                    {isLoading ? 'Creating...' : 'Create Party'}
                  </Button>
                </Card>

                {/* Join Party */}
                <Card>
                  <h2 className="text-lg font-semibold mb-4">Join a Party</h2>
                  <p className="text-text-secondary text-sm mb-4">
                    Enter the party code shared by your squad leader.
                  </p>
                  <div className="flex gap-2">
                    <input
                      type="text"
                      value={joinCode}
                      onChange={(e) => setJoinCode(e.target.value.toUpperCase())}
                      placeholder="ALPHA-1234"
                      className="flex-1 glass-input px-3 py-2 text-white placeholder-text-muted"
                    />
                    <Button onClick={handleJoinParty} disabled={isLoading || !joinCode.trim()}>
                      <ArrowRightOnRectangleIcon className="h-5 w-5" />
                      Join
                    </Button>
                  </div>
                </Card>
              </>
            ) : (
              <>
                {/* Active Party */}
                <Card>
                  <div className="flex items-center justify-between mb-4">
                    <h2 className="text-lg font-semibold">
                      {status.isHost ? 'Your Party' : 'In Party'}
                    </h2>
                    <span className="bg-neon-green/20 text-neon-green px-2 py-1 rounded text-sm">
                      {status.memberCount} / 5 members
                    </span>
                  </div>

                  {/* Party Code */}
                  <div className="glass-panel p-4 mb-4">
                    <div className="text-text-secondary text-sm mb-1">Party Code</div>
                    <div className="flex items-center justify-between">
                      <span className="text-2xl font-mono font-bold tracking-wider text-primary-purple">
                        {status.partyCode}
                      </span>
                      <Button size="sm" variant="secondary" onClick={handleCopyCode} title="Copy party code">
                        {copied ? (
                          <>
                            <CheckIcon className="h-4 w-4 text-neon-green" />
                            Copied!
                          </>
                        ) : (
                          <>
                            <ClipboardDocumentIcon className="h-4 w-4" />
                            Copy
                          </>
                        )}
                      </Button>
                    </div>
                  </div>

                  <Button variant="danger" onClick={handleLeaveParty} disabled={isLoading} className="w-full">
                    {isLoading ? 'Leaving...' : 'Leave Party'}
                  </Button>
                </Card>
              </>
            )}
          </div>

          {/* Right: Members List */}
          <Card>
            <h2 className="text-lg font-semibold mb-4">
              Party Members
            </h2>

            {!status.inParty ? (
              <div className="text-center py-8 text-text-muted">
                <UserGroupIcon className="h-12 w-12 mx-auto mb-3 opacity-50" />
                <p>Join or create a party to see members</p>
              </div>
            ) : members.length === 0 ? (
              <div className="text-center py-8 text-text-muted">
                <p>No members yet</p>
              </div>
            ) : (
              <div className="space-y-3">
                {members.map((member) => (
                  <div
                    key={member.clientId}
                    className="flex items-center justify-between glass-panel p-3"
                  >
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 bg-bg-hover rounded-full flex items-center justify-center">
                        {member.displayName.charAt(0).toUpperCase()}
                      </div>
                      <div>
                        <div className="font-medium flex items-center gap-2">
                          {member.displayName}
                          {member.isHost && (
                            <span className="bg-warning/20 text-warning text-xs px-1.5 py-0.5 rounded">
                              Host
                            </span>
                          )}
                          {member.clientId === status.clientId && (
                            <span className="text-text-muted text-xs">(You)</span>
                          )}
                        </div>
                        {member.currentMap && (
                          <div className="text-sm text-text-secondary flex items-center gap-1">
                            <MapPinIcon className="h-3 w-3" />
                            {member.currentMap}
                          </div>
                        )}
                      </div>
                    </div>

                    {/* Add Friend button (if not self and not already friend) */}
                    {member.clientId !== status.clientId && !friends.some(f => f.clientId === member.clientId) && (
                      outgoingFriendRequests.some(r => r.clientId === member.clientId) ? (
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => handleCancelFriendRequest(member.clientId)}
                          title="Cancel friend request"
                          className="text-yellow-400 border-yellow-500/30 hover:bg-yellow-500/20"
                        >
                          <XMarkIcon className="h-4 w-4" />
                          Cancel Request
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => handleAddFriend(member.clientId)}
                          title="Send friend request"
                        >
                          <UserPlusIcon className="h-4 w-4" />
                          Add Friend
                        </Button>
                      )
                    )}
                  </div>
                ))}
              </div>
            )}
          </Card>
        </div>
      )}

      {activeTab === 'friends' && (
        <div className="space-y-6">
          {/* Incoming Friend Requests */}
          {incomingFriendRequests.length > 0 && (
            <Card>
              <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
                <BellIcon className="h-5 w-5 text-primary-purple" />
                Friend Requests ({incomingFriendRequests.length})
              </h2>
              <div className="space-y-3">
                {incomingFriendRequests.map((request) => (
                  <div
                    key={request.clientId}
                    className="flex items-center justify-between glass-panel p-3"
                  >
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 bg-bg-hover rounded-full flex items-center justify-center">
                        {request.displayName.charAt(0).toUpperCase()}
                      </div>
                      <div>
                        <div className="font-medium">{request.displayName}</div>
                        <div className="text-sm text-text-secondary">Wants to be friends!</div>
                      </div>
                    </div>
                    <div className="flex gap-2">
                      <Button size="sm" onClick={() => handleAcceptFriendRequest(request.clientId)} title="Accept friend request">
                        <CheckIcon className="h-4 w-4" />
                        Accept
                      </Button>
                      <Button size="sm" variant="secondary" onClick={() => handleDeclineFriendRequest(request.clientId)} title="Decline friend request">
                        <XMarkIcon className="h-4 w-4" />
                        Decline
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            </Card>
          )}

          {/* Outgoing Friend Requests */}
          {outgoingFriendRequests.length > 0 && (
            <Card>
              <h2 className="text-lg font-semibold mb-4 text-text-secondary">
                Pending Requests ({outgoingFriendRequests.length})
              </h2>
              <div className="space-y-3">
                {outgoingFriendRequests.map((request) => (
                  <div
                    key={request.clientId}
                    className="flex items-center justify-between glass-panel p-3"
                  >
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 bg-bg-hover rounded-full flex items-center justify-center">
                        {request.displayName.charAt(0).toUpperCase()}
                      </div>
                      <div>
                        <div className="font-medium">{request.displayName}</div>
                        <div className="text-sm text-text-secondary">Request pending.</div>
                      </div>
                    </div>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => handleCancelFriendRequest(request.clientId)}
                      title="Cancel friend request"
                      className="text-yellow-400 border-yellow-500/30 hover:bg-yellow-500/20"
                    >
                      <XMarkIcon className="h-4 w-4" />
                      Cancel Request
                    </Button>
                  </div>
                ))}
              </div>
            </Card>
          )}

          {/* Friends List */}
          <Card>
            <h2 className="text-lg font-semibold mb-4">Friends</h2>

            {friends.length === 0 ? (
            <div className="text-center py-8 text-text-muted">
              <UserGroupIcon className="h-12 w-12 mx-auto mb-3 opacity-50" />
              <p>No friends yet</p>
              <p className="text-sm mt-2">Add party members as friends to see them here</p>
            </div>
          ) : (
            <div className="space-y-3">
              {/* Online friends first */}
              {friends
                .sort((a, b) => (b.online ? 1 : 0) - (a.online ? 1 : 0))
                .map((friend) => (
                  <div
                    key={friend.clientId}
                    className="flex items-center justify-between glass-panel p-3"
                  >
                    <div className="flex items-center gap-3">
                      <div className="relative">
                        <div className="w-8 h-8 bg-bg-hover rounded-full flex items-center justify-center">
                          {friend.displayName.charAt(0).toUpperCase()}
                        </div>
                        <span
                          className={`absolute bottom-0 right-0 w-2.5 h-2.5 rounded-full border-2 border-bg-card ${
                            friend.online ? 'bg-neon-green' : 'bg-text-muted'
                          }`}
                        />
                      </div>
                      <div>
                        <div className="font-medium">{friend.displayName}</div>
                        <div className="text-sm text-text-secondary">
                          {friend.online
                            ? friend.inParty
                              ? `In Party: ${friend.partyCode}`
                              : 'Online'
                            : 'Offline'}
                        </div>
                      </div>
                    </div>

                    <div className="flex gap-2">
                      {/* Invite to party button (if online and we're in a party and not already invited) */}
                      {friend.online && status.inParty && !friend.inParty && !sentInvites.has(friend.clientId) && (
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => handleInviteFriend(friend.clientId)}
                          title="Invite to your party"
                        >
                          <UserPlusIcon className="h-4 w-4" />
                          Invite
                        </Button>
                      )}

                      {/* Cancel invite button (if we've sent an invite) */}
                      {friend.online && status.inParty && !friend.inParty && sentInvites.has(friend.clientId) && (
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => handleCancelInvite(friend.clientId)}
                          title="Cancel party invite"
                          className="text-yellow-400 border-yellow-500/30 hover:bg-yellow-500/20"
                        >
                          <XMarkIcon className="h-4 w-4" />
                          Cancel Invite
                        </Button>
                      )}

                      {/* Join their party button */}
                      {friend.online && friend.inParty && !status.inParty && friend.partyCode && (
                        <Button
                          size="sm"
                          onClick={async () => {
                            setIsLoading(true);
                            setError('');
                            try {
                              await JoinCentralParty(friend.partyCode!.trim().toUpperCase());
                            } catch (err: any) {
                              setError(err.message || 'Failed to join party');
                            } finally {
                              setIsLoading(false);
                            }
                          }}
                          disabled={isLoading}
                          title="Join their party"
                        >
                          <ArrowRightOnRectangleIcon className="h-4 w-4" />
                          Join Party
                        </Button>
                      )}

                      {/* Remove friend */}
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => handleRemoveFriend(friend.clientId)}
                        title="Remove from friends list"
                        className="text-error hover:text-error/80"
                      >
                        <UserMinusIcon className="h-4 w-4" />
                        Remove
                      </Button>
                    </div>
                  </div>
                ))}
            </div>
          )}
          </Card>
        </div>
      )}

      {/* Info Box */}
      <Card className="mt-6">
        <h3 className="font-semibold text-sm mb-2">How it works</h3>
        <ul className="text-sm text-text-secondary space-y-1">
          <li>1. Create a party or join one with a code</li>
          <li>2. Your position is shared with party members in real-time</li>
          <li>3. Add party members as friends for quick invites next time</li>
          <li>4. Friends can see when you're online and in a party</li>
        </ul>
      </Card>
    </div>
  );
}
