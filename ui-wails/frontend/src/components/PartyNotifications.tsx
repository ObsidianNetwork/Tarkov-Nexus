import React, { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { useToast } from './ToastNotification';
import {
  AcceptFriendRequest,
  DeclineFriendRequest,
  AcceptPartyInvite,
  DeclinePartyInvite,
} from '../../wailsjs/go/main/App';
import {
  CheckIcon,
  XMarkIcon,
  ArrowRightOnRectangleIcon,
} from '@heroicons/react/24/outline';

interface FriendRequestData {
  clientId: string;
  displayName: string;
}

interface PartyInviteData {
  fromClientId: string;
  fromDisplayName: string;
  partyCode: string;
}

interface FriendAcceptedData {
  clientId: string;
  displayName: string;
}

export function PartyNotifications() {
  const { addToast } = useToast();
  const navigate = useNavigate();

  useEffect(() => {
    // Friend request received
    const unsubFriendRequest = EventsOn('centralParty:friendRequest', (data: FriendRequestData) => {
      addToast({
        type: 'friendRequest',
        title: 'Friend Request',
        message: `${data.displayName} wants to be friends!`,
        duration: 15000, // Longer duration for actionable notifications
        actions: [
          {
            label: 'Accept',
            icon: React.createElement(CheckIcon, { className: 'h-4 w-4' }),
            onClick: async () => {
              try {
                await AcceptFriendRequest(data.clientId);
              } catch (e) {
                console.error('Failed to accept friend request:', e);
              }
            },
            variant: 'primary',
          },
          {
            label: 'Decline',
            icon: React.createElement(XMarkIcon, { className: 'h-4 w-4' }),
            onClick: async () => {
              try {
                await DeclineFriendRequest(data.clientId);
              } catch (e) {
                console.error('Failed to decline friend request:', e);
              }
            },
            variant: 'secondary',
          },
        ],
        data: data,
      });
    });

    // Friend request accepted (someone accepted yours)
    const unsubFriendAccepted = EventsOn('centralParty:friendRequestAccepted', (data: FriendAcceptedData) => {
      addToast({
        type: 'success',
        title: 'Friend Added!',
        message: `${data.displayName} accepted your friend request.`,
        duration: 5000,
      });
    });

    // Friend request declined (someone declined yours)
    const unsubFriendDeclined = EventsOn('centralParty:friendRequestDeclined', (data: FriendAcceptedData) => {
      addToast({
        type: 'info',
        title: 'Request Declined',
        message: `${data.displayName} declined your friend request.`,
        duration: 5000,
      });
    });

    // Party invite received
    const unsubPartyInvite = EventsOn('centralParty:invite', (data: PartyInviteData) => {
      addToast({
        type: 'partyInvite',
        title: 'Party Invite',
        message: `${data.fromDisplayName} invited you to their party!`,
        duration: 20000, // Longer for party invites
        actions: [
          {
            label: 'Join Party',
            icon: React.createElement(ArrowRightOnRectangleIcon, { className: 'h-4 w-4' }),
            onClick: async () => {
              try {
                await AcceptPartyInvite(data.partyCode);
                navigate('/party');
              } catch (e) {
                console.error('Failed to accept party invite:', e);
              }
            },
            variant: 'primary',
          },
          {
            label: 'Decline',
            icon: React.createElement(XMarkIcon, { className: 'h-4 w-4' }),
            onClick: async () => {
              try {
                await DeclinePartyInvite(data.partyCode);
              } catch (e) {
                console.error('Failed to decline party invite:', e);
              }
            },
            variant: 'secondary',
          },
        ],
        data: data,
      });
    });

    // Invite accepted (someone joined your party)
    const unsubInviteAccepted = EventsOn('centralParty:inviteAccepted', (data: FriendAcceptedData) => {
      addToast({
        type: 'success',
        title: 'Invite Accepted',
        message: `${data.displayName} joined your party!`,
        duration: 5000,
      });
    });

    // Invite declined
    const unsubInviteDeclined = EventsOn('centralParty:inviteDeclined', (data: FriendAcceptedData) => {
      addToast({
        type: 'info',
        title: 'Invite Declined',
        message: `${data.displayName} declined your party invite.`,
        duration: 5000,
      });
    });

    // Member joined party
    const unsubMemberJoined = EventsOn('centralParty:memberJoined', (data: { clientId: string; displayName: string }) => {
      addToast({
        type: 'info',
        title: 'Member Joined',
        message: `${data.displayName} joined the party.`,
        duration: 4000,
      });
    });

    // Member left party
    const unsubMemberLeft = EventsOn('centralParty:memberLeft', (data: { clientId: string }) => {
      addToast({
        type: 'info',
        title: 'Member Left',
        message: 'A member left the party.',
        duration: 4000,
      });
    });

    // Friend came online
    const unsubFriendOnline = EventsOn('centralParty:friendOnline', (data: FriendAcceptedData) => {
      addToast({
        type: 'info',
        title: 'Friend Online',
        message: `${data.displayName} is now online.`,
        duration: 4000,
      });
    });

    // Cleanup
    return () => {
      if (typeof unsubFriendRequest === 'function') unsubFriendRequest();
      if (typeof unsubFriendAccepted === 'function') unsubFriendAccepted();
      if (typeof unsubFriendDeclined === 'function') unsubFriendDeclined();
      if (typeof unsubPartyInvite === 'function') unsubPartyInvite();
      if (typeof unsubInviteAccepted === 'function') unsubInviteAccepted();
      if (typeof unsubInviteDeclined === 'function') unsubInviteDeclined();
      if (typeof unsubMemberJoined === 'function') unsubMemberJoined();
      if (typeof unsubMemberLeft === 'function') unsubMemberLeft();
      if (typeof unsubFriendOnline === 'function') unsubFriendOnline();
    };
  }, [addToast, navigate]);

  // This component doesn't render anything visible
  return null;
}

export default PartyNotifications;
